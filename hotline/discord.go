package hotline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultDiscordAPIBaseURL = "https://discord.com/api/v10"

// DiscordConfig configures the Discord REST transport.
type DiscordConfig struct {
	BotToken       string
	ChannelID      string
	APIBaseURL     string
	UserAgent      string
	HTTPClient     *http.Client
	AcceptAnyAfter bool
	AllowAnyUser   bool
}

// DiscordAdapter posts a channel message and polls for an authorized reply.
type DiscordAdapter struct {
	config DiscordConfig
	client *http.Client
}

func NewDiscordAdapter(config DiscordConfig) (*DiscordAdapter, error) {
	config.BotToken = strings.TrimSpace(config.BotToken)
	config.ChannelID = strings.TrimSpace(config.ChannelID)
	config.APIBaseURL = strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	config.UserAgent = strings.TrimSpace(config.UserAgent)
	if config.BotToken == "" {
		return nil, errors.New("discord bot token is required")
	}
	if config.ChannelID == "" {
		return nil, errors.New("discord channel id is required")
	}
	if config.APIBaseURL == "" {
		config.APIBaseURL = defaultDiscordAPIBaseURL
	}
	if config.UserAgent == "" {
		config.UserAgent = "anvil-hotline/0"
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &DiscordAdapter{config: config, client: client}, nil
}

func (a *DiscordAdapter) Ask(ctx context.Context, question Question) (Response, error) {
	if len(normalizedDiscordUserIDs(question.AllowedUserIDs)) == 0 && !a.config.AllowAnyUser {
		return Response{}, errors.New("at least one allowed Discord user id is required unless allow-any-user is explicitly enabled")
	}
	reactions, err := normalizeReactionOptions(question.Reactions)
	if err != nil {
		return Response{}, err
	}
	question.Reactions = reactions
	attachments, err := normalizeAttachments(question.Attachments)
	if err != nil {
		return Response{}, err
	}
	question.Attachments = attachments
	if question.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, question.Timeout)
		defer cancel()
	}
	prompt, err := a.createMessage(ctx, FormatQuestionMessage(question), question.Attachments)
	if err != nil {
		return Response{}, err
	}
	if err := a.applyReactions(ctx, prompt.ID, question.Reactions); err != nil {
		return Response{}, err
	}
	response, err := a.waitForPrimaryAnswer(ctx, prompt.ID, question)
	if err != nil {
		return Response{}, err
	}
	if response.Source == "reply" {
		// Free-text is the full critique; no second wait.
		response.Choice = ""
		return response, nil
	}
	if response.Source == "reaction" {
		response.Choice = response.Text
	}
	if !shouldCollectNotes(question, response) {
		return response, nil
	}
	return a.collectNotesAfterReaction(ctx, prompt.ID, question, response)
}

func (a *DiscordAdapter) createMessage(ctx context.Context, content string, attachments []Attachment) (discordMessage, error) {
	if len(attachments) == 0 {
		return a.createJSONMessage(ctx, content)
	}
	return a.createMultipartMessage(ctx, content, attachments)
}

func (a *DiscordAdapter) createJSONMessage(ctx context.Context, content string) (discordMessage, error) {
	var message discordMessage
	request := discordCreateMessageRequest{
		Content: content,
		AllowedMentions: discordAllowedMentions{
			Parse: []string{},
		},
	}
	if err := a.doJSON(ctx, http.MethodPost, "/channels/"+url.PathEscape(a.config.ChannelID)+"/messages", nil, request, &message); err != nil {
		return discordMessage{}, fmt.Errorf("post discord question: %w", err)
	}
	if strings.TrimSpace(message.ID) == "" {
		return discordMessage{}, errors.New("discord create message response did not include message id")
	}
	return message, nil
}

func (a *DiscordAdapter) createMultipartMessage(ctx context.Context, content string, attachments []Attachment) (discordMessage, error) {
	payload := discordCreateMessageRequest{
		Content: content,
		AllowedMentions: discordAllowedMentions{
			Parse: []string{},
		},
		Attachments: make([]discordAttachmentMeta, 0, len(attachments)),
	}
	for i, attachment := range attachments {
		payload.Attachments = append(payload.Attachments, discordAttachmentMeta{
			ID:       i,
			Filename: attachment.Filename,
		})
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return discordMessage{}, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	payloadPart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="payload_json"`},
		"Content-Type":        []string{"application/json"},
	})
	if err != nil {
		return discordMessage{}, err
	}
	if _, err := payloadPart.Write(payloadJSON); err != nil {
		return discordMessage{}, err
	}

	for i, attachment := range attachments {
		data, info, err := readAttachmentFile(attachment)
		if err != nil {
			return discordMessage{}, err
		}
		_ = info
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(
			`form-data; name="files[%d]"; filename="%s"`,
			i,
			escapeMultipartFilename(attachment.Filename),
		))
		header.Set("Content-Type", attachment.ContentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return discordMessage{}, err
		}
		if _, err := part.Write(data); err != nil {
			return discordMessage{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return discordMessage{}, err
	}

	var message discordMessage
	if err := a.doMultipart(ctx, http.MethodPost, "/channels/"+url.PathEscape(a.config.ChannelID)+"/messages", writer.FormDataContentType(), body.Bytes(), &message); err != nil {
		return discordMessage{}, fmt.Errorf("post discord question with attachments: %w", err)
	}
	if strings.TrimSpace(message.ID) == "" {
		return discordMessage{}, errors.New("discord create message response did not include message id")
	}
	return message, nil
}

func readAttachmentFile(attachment Attachment) ([]byte, os.FileInfo, error) {
	// Path is operator-supplied via CLI/library; not concatenated from untrusted input.
	info, err := os.Stat(attachment.Path) // #nosec G304 -- intentional operator-supplied path
	if err != nil {
		return nil, nil, fmt.Errorf("attachment %q: %w", attachment.Filename, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("attachment %q is not a regular file", attachment.Filename)
	}
	if info.Size() <= 0 {
		return nil, nil, fmt.Errorf("attachment %q is empty", attachment.Filename)
	}
	if info.Size() > MaxAttachmentBytes {
		return nil, nil, fmt.Errorf("attachment %q exceeds %d bytes", attachment.Filename, MaxAttachmentBytes)
	}
	data, err := os.ReadFile(attachment.Path) // #nosec G304 -- intentional operator-supplied path
	if err != nil {
		return nil, nil, fmt.Errorf("read attachment %q: %w", attachment.Filename, err)
	}
	return data, info, nil
}

func escapeMultipartFilename(filename string) string {
	// Keep basename only and strip quotes that would break the disposition header.
	base := filepath.Base(filename)
	base = strings.ReplaceAll(base, `"`, "")
	base = strings.ReplaceAll(base, "\r", "")
	base = strings.ReplaceAll(base, "\n", "")
	if base == "" || base == "." {
		return "attachment.bin"
	}
	return base
}

func (a *DiscordAdapter) doMultipart(ctx context.Context, method, path, contentType string, body []byte, out any) error {
	for attempt := 0; ; attempt++ {
		endpoint := a.config.APIBaseURL + path
		request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bot "+a.config.BotToken)
		request.Header.Set("User-Agent", a.config.UserAgent)
		request.Header.Set("Content-Type", contentType)
		response, err := a.client.Do(request)
		if err != nil {
			return err
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt < 5 {
			delay := discordRetryDelay(response.Body, response.Header.Get("Retry-After"))
			response.Body.Close()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode > 299 {
			respBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			return fmt.Errorf("discord API %s %s returned %s: %s", method, path, response.Status, strings.TrimSpace(string(respBody)))
		}
		if out == nil {
			return nil
		}
		if err := json.NewDecoder(response.Body).Decode(out); err != nil {
			return err
		}
		return nil
	}
}

func (a *DiscordAdapter) applyReactions(ctx context.Context, messageID string, reactions []ReactionOption) error {
	for _, reaction := range reactions {
		if err := a.createReaction(ctx, messageID, reaction.Emoji); err != nil {
			return fmt.Errorf("apply reaction %q: %w", reaction.Emoji, err)
		}
	}
	return nil
}

func (a *DiscordAdapter) createReaction(ctx context.Context, messageID, emoji string) error {
	path := "/channels/" + url.PathEscape(a.config.ChannelID) +
		"/messages/" + url.PathEscape(messageID) +
		"/reactions/" + discordReactionPathEmoji(emoji) + "/@me"
	if err := a.doJSON(ctx, http.MethodPut, path, nil, nil, nil); err != nil {
		return fmt.Errorf("create discord reaction: %w", err)
	}
	return nil
}

func (a *DiscordAdapter) waitForPrimaryAnswer(ctx context.Context, questionMessageID string, question Question) (Response, error) {
	pollInterval := question.PollInterval
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	acceptAnyAfter := question.AcceptAnyAfter || a.config.AcceptAnyAfter
	for {
		messages, err := a.channelMessagesAfter(ctx, questionMessageID)
		if err != nil {
			return Response{}, err
		}
		if message, ok := selectDiscordReply(messages, questionMessageID, a.config.ChannelID, question.AllowedUserIDs, a.config.AllowAnyUser, acceptAnyAfter); ok {
			return Response{
				Text:           strings.TrimSpace(message.Content),
				Source:         "reply",
				AuthorID:       message.Author.ID,
				AuthorUsername: message.Author.Username,
				MessageID:      message.ID,
				ChannelID:      message.ChannelID,
			}, nil
		}
		if response, ok, err := a.selectDiscordReaction(ctx, questionMessageID, question); err != nil {
			return Response{}, err
		} else if ok {
			return response, nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{}, fmt.Errorf("wait for discord reply: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// collectNotesAfterReaction optionally waits for free-text after a reaction.
// Never required: if the human only reacts, return that reaction as the full answer.
func (a *DiscordAdapter) collectNotesAfterReaction(ctx context.Context, questionMessageID string, question Question, primary Response) (Response, error) {
	pollInterval := question.PollInterval
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	acceptAnyAfter := question.AcceptAnyAfter || a.config.AcceptAnyAfter

	timeout := question.NotesTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	notesCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		messages, err := a.channelMessagesAfter(notesCtx, questionMessageID)
		if err != nil {
			if notesCtx.Err() != nil {
				return primary, nil
			}
			return Response{}, err
		}
		if message, ok := selectDiscordReply(messages, questionMessageID, a.config.ChannelID, question.AllowedUserIDs, a.config.AllowAnyUser, acceptAnyAfter); ok {
			notes := strings.TrimSpace(message.Content)
			if notes != "" {
				primary.Notes = notes
				primary.Source = "reaction+notes"
				return primary, nil
			}
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-notesCtx.Done():
			timer.Stop()
			// Reaction alone is a complete answer.
			return primary, nil
		case <-timer.C:
		}
	}
}

func (a *DiscordAdapter) selectDiscordReaction(ctx context.Context, questionMessageID string, question Question) (Response, bool, error) {
	if len(question.Reactions) == 0 {
		return Response{}, false, nil
	}
	allowed := normalizedDiscordUserIDs(question.AllowedUserIDs)
	if len(allowed) == 0 && !a.config.AllowAnyUser {
		return Response{}, false, nil
	}
	for _, reaction := range question.Reactions {
		users, err := a.reactionUsers(ctx, questionMessageID, reaction.Emoji)
		if err != nil {
			return Response{}, false, err
		}
		if user, ok := selectAuthorizedReactionUser(users, allowed, a.config.AllowAnyUser); ok {
			return Response{
				Text:           reaction.Value,
				Source:         "reaction",
				AuthorID:       user.ID,
				AuthorUsername: user.Username,
				MessageID:      questionMessageID,
				ChannelID:      a.config.ChannelID,
				ReactionEmoji:  reaction.Emoji,
			}, true, nil
		}
	}
	return Response{}, false, nil
}

func (a *DiscordAdapter) reactionUsers(ctx context.Context, messageID, emoji string) ([]discordUser, error) {
	values := url.Values{}
	values.Set("limit", "100")
	path := "/channels/" + url.PathEscape(a.config.ChannelID) +
		"/messages/" + url.PathEscape(messageID) +
		"/reactions/" + discordReactionPathEmoji(emoji)
	var users []discordUser
	if err := a.doJSON(ctx, http.MethodGet, path, values, nil, &users); err != nil {
		return nil, fmt.Errorf("poll discord reactions for %q: %w", emoji, err)
	}
	return users, nil
}

// discordReactionPathEmoji encodes an emoji for Discord reaction path segments.
// Unicode emojis are percent-encoded; custom emojis use name:id.
func discordReactionPathEmoji(emoji string) string {
	return url.PathEscape(strings.TrimSpace(emoji))
}

func selectAuthorizedReactionUser(users []discordUser, allowed map[string]struct{}, allowAnyUser bool) (discordUser, bool) {
	for _, user := range users {
		if user.Bot || strings.TrimSpace(user.ID) == "" {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[user.ID]; !ok {
				continue
			}
		} else if !allowAnyUser {
			continue
		}
		return user, true
	}
	return discordUser{}, false
}

func (a *DiscordAdapter) channelMessagesAfter(ctx context.Context, messageID string) ([]discordMessage, error) {
	values := url.Values{}
	values.Set("after", messageID)
	values.Set("limit", "50")
	var messages []discordMessage
	if err := a.doJSON(ctx, http.MethodGet, "/channels/"+url.PathEscape(a.config.ChannelID)+"/messages", values, nil, &messages); err != nil {
		return nil, fmt.Errorf("poll discord replies: %w", err)
	}
	return messages, nil
}

func normalizedDiscordUserIDs(allowedUserIDs []string) map[string]struct{} {
	allowed := map[string]struct{}{}
	for _, id := range allowedUserIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	return allowed
}

func selectDiscordReply(messages []discordMessage, questionMessageID, channelID string, allowedUserIDs []string, allowAnyUser, acceptAnyAfter bool) (discordMessage, bool) {
	allowed := normalizedDiscordUserIDs(allowedUserIDs)
	if len(allowed) == 0 && !allowAnyUser {
		return discordMessage{}, false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Author.Bot || strings.TrimSpace(message.Content) == "" {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[message.Author.ID]; !ok {
				continue
			}
		}
		if !acceptAnyAfter && !discordMessageReferences(message, questionMessageID, channelID) {
			continue
		}
		return message, true
	}
	return discordMessage{}, false
}

func discordMessageReferences(message discordMessage, questionMessageID, channelID string) bool {
	if message.MessageReference == nil {
		return false
	}
	if message.MessageReference.MessageID != questionMessageID {
		return false
	}
	return message.MessageReference.ChannelID == "" || message.MessageReference.ChannelID == channelID
}

func (a *DiscordAdapter) doJSON(ctx context.Context, method, path string, query url.Values, in any, out any) error {
	for attempt := 0; ; attempt++ {
		var body io.Reader
		if in != nil {
			payload, err := json.Marshal(in)
			if err != nil {
				return err
			}
			body = bytes.NewReader(payload)
		}
		endpoint := a.config.APIBaseURL + path
		if len(query) > 0 {
			endpoint += "?" + query.Encode()
		}
		request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bot "+a.config.BotToken)
		request.Header.Set("User-Agent", a.config.UserAgent)
		if in != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := a.client.Do(request)
		if err != nil {
			return err
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt < 5 {
			delay := discordRetryDelay(response.Body, response.Header.Get("Retry-After"))
			response.Body.Close()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode > 299 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			return fmt.Errorf("discord API %s %s returned %s: %s", method, path, response.Status, strings.TrimSpace(string(body)))
		}
		if out == nil {
			return nil
		}
		if err := json.NewDecoder(response.Body).Decode(out); err != nil {
			return err
		}
		return nil
	}
}

func discordRetryDelay(body io.Reader, retryAfterHeader string) time.Duration {
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err == nil && payload.RetryAfter > 0 {
		return time.Duration(payload.RetryAfter * float64(time.Second))
	}
	if retryAfterHeader != "" {
		if delay, err := time.ParseDuration(retryAfterHeader + "s"); err == nil {
			return delay
		}
	}
	return time.Second
}

type discordCreateMessageRequest struct {
	Content         string                  `json:"content"`
	AllowedMentions discordAllowedMentions  `json:"allowed_mentions"`
	Attachments     []discordAttachmentMeta `json:"attachments,omitempty"`
}

type discordAttachmentMeta struct {
	ID       int    `json:"id"`
	Filename string `json:"filename"`
}

type discordAllowedMentions struct {
	Parse []string `json:"parse"`
}

type discordMessage struct {
	ID               string                   `json:"id"`
	ChannelID        string                   `json:"channel_id"`
	Content          string                   `json:"content"`
	Author           discordUser              `json:"author"`
	MessageReference *discordMessageReference `json:"message_reference"`
}

type discordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

type discordMessageReference struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
}
