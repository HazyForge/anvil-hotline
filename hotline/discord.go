package hotline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const defaultDiscordAPIBaseURL = "https://discord.com/api/v10"

// DiscordConfig configures the Discord REST transport.
type DiscordConfig struct {
	BotToken  string
	ChannelID string
	// ParentChannelID, when set, requires ChannelID to be a Discord thread
	// directly under this parent. It prevents --thread-id from becoming an
	// arbitrary channel read/write primitive.
	ParentChannelID string
	APIBaseURL      string
	UserAgent       string
	HTTPClient      *http.Client
	AcceptAnyAfter  bool
	AllowAnyUser    bool
}

// DiscordAdapter posts a channel message and polls for an authorized reply.
type DiscordAdapter struct {
	config DiscordConfig
	client *http.Client
}

func NewDiscordAdapter(config DiscordConfig) (*DiscordAdapter, error) {
	config.BotToken = strings.TrimSpace(config.BotToken)
	config.ChannelID = strings.TrimSpace(config.ChannelID)
	config.ParentChannelID = strings.TrimSpace(config.ParentChannelID)
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
	if a.config.ParentChannelID != "" {
		if _, err := a.scopedThread(ctx, a.config.ChannelID); err != nil {
			return Response{}, err
		}
	}
	if len(normalizedDiscordUserIDs(question.AllowedUserIDs)) == 0 && !a.config.AllowAnyUser {
		return Response{}, errors.New("at least one allowed Discord user id is required unless allow-any-user is explicitly enabled")
	}
	reactions, err := normalizeReactionOptions(question.Reactions)
	if err != nil {
		return Response{}, err
	}
	question.Reactions = reactions
	if question.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, question.Timeout)
		defer cancel()
	}
	var prompt discordMessage
	marker := discordQuestionMarker(question.IdempotencyKey)
	payloadMarker := discordQuestionPayloadMarker(question)
	content := FormatQuestionMessage(question)
	if marker != "" {
		bot, botErr := a.currentUser(ctx)
		if botErr != nil {
			return Response{}, botErr
		}
		var found bool
		prompt, found, err = a.findQuestionMessage(ctx, marker, payloadMarker, bot.ID)
		if err != nil {
			return Response{}, err
		}
		if !found {
			prompt, err = a.createMessage(ctx, content, discordQuestionNonce(a.config.ChannelID, question.IdempotencyKey))
			if err == nil && !strings.Contains(prompt.Content, payloadMarker) {
				return Response{}, errors.New("idempotency key resolved to a discord message with different question content")
			}
		}
	} else {
		prompt, err = a.createMessage(ctx, content, "")
	}
	if err != nil {
		return Response{}, err
	}
	if err := a.applyReactions(ctx, prompt.ID, question.Reactions); err != nil {
		return Response{}, err
	}
	response, err := a.waitForReply(ctx, prompt.ID, question)
	response.QuestionMessageID = prompt.ID
	return response, err
}

func (a *DiscordAdapter) currentUser(ctx context.Context) (discordUser, error) {
	var user discordUser
	if err := a.doJSON(ctx, http.MethodGet, "/users/@me", nil, nil, &user); err != nil {
		return discordUser{}, fmt.Errorf("read authenticated discord bot identity: %w", err)
	}
	if strings.TrimSpace(user.ID) == "" || !user.Bot {
		return discordUser{}, errors.New("discord authenticated user is not a bot with an id")
	}
	return user, nil
}

func (a *DiscordAdapter) findQuestionMessage(ctx context.Context, marker, payloadMarker, botID string) (discordMessage, bool, error) {
	return a.findBotMessageInChannel(ctx, a.config.ChannelID, marker, payloadMarker, botID)
}

func (a *DiscordAdapter) findBotMessageInChannel(ctx context.Context, channelID, marker, payloadMarker, botID string) (discordMessage, bool, error) {
	before := ""
	for {
		values := url.Values{}
		values.Set("limit", "100")
		if before != "" {
			values.Set("before", before)
		}
		var messages []discordMessage
		if err := a.doJSON(ctx, http.MethodGet, "/channels/"+url.PathEscape(channelID)+"/messages", values, nil, &messages); err != nil {
			return discordMessage{}, false, fmt.Errorf("find existing discord question: %w", err)
		}
		for _, message := range messages {
			if !message.Author.Bot || message.Author.ID != botID || !strings.Contains(message.Content, marker) {
				continue
			}
			if !strings.Contains(message.Content, payloadMarker) {
				return discordMessage{}, false, errors.New("idempotency key was already used by this bot for different question content")
			}
			return message, true, nil
		}
		if len(messages) < 100 {
			return discordMessage{}, false, nil
		}
		before = strings.TrimSpace(messages[len(messages)-1].ID)
		if before == "" {
			return discordMessage{}, false, errors.New("discord history page did not include an oldest message id")
		}
	}
}

func (a *DiscordAdapter) createMessage(ctx context.Context, content, nonce string) (discordMessage, error) {
	return a.createMessageInChannel(ctx, a.config.ChannelID, content, nonce)
}

func (a *DiscordAdapter) createMessageInChannel(ctx context.Context, channelID, content, nonce string) (discordMessage, error) {
	var message discordMessage
	request := discordCreateMessageRequest{
		Content: content,
		AllowedMentions: discordAllowedMentions{
			Parse: []string{},
		},
	}
	if nonce != "" {
		request.Nonce = nonce
		request.EnforceNonce = true
	}
	if err := a.doJSON(ctx, http.MethodPost, "/channels/"+url.PathEscape(channelID)+"/messages", nil, request, &message); err != nil {
		return discordMessage{}, fmt.Errorf("post discord question: %w", err)
	}
	if strings.TrimSpace(message.ID) == "" {
		return discordMessage{}, errors.New("discord create message response did not include message id")
	}
	return message, nil
}

func (a *DiscordAdapter) OpenThread(ctx context.Context, request ThreadRequest) (Thread, error) {
	if len(normalizedDiscordUserIDs(request.AllowedUserIDs)) == 0 && !a.config.AllowAnyUser {
		return Thread{}, errors.New("at least one allowed Discord user id is required unless allow-any-user is explicitly enabled")
	}
	bot, err := a.currentUser(ctx)
	if err != nil {
		return Thread{}, err
	}
	marker := discordThreadMarker(request.IdempotencyKey)
	payloadMarker := discordThreadPayloadMarker(request)
	prompt, found, err := a.findBotMessageInChannel(ctx, a.config.ChannelID, marker, payloadMarker, bot.ID)
	if err != nil {
		return Thread{}, err
	}
	if !found {
		content := FormatThreadStarterMessage(request)
		prompt, err = a.createMessageInChannel(ctx, a.config.ChannelID, content, discordQuestionNonce(a.config.ChannelID, "thread:"+request.IdempotencyKey))
		if err != nil {
			return Thread{}, err
		}
		if !strings.Contains(prompt.Content, payloadMarker) {
			return Thread{}, errors.New("thread idempotency key resolved to a Discord message with different content")
		}
	}
	if prompt.Thread != nil {
		return a.threadFromChannel(*prompt.Thread, prompt.ID)
	}
	// Discord assigns a message-started thread the same snowflake as its source
	// message. This makes a retry recoverable even if the process died after the
	// thread was created but before its response was persisted.
	if existing, getErr := a.getChannel(ctx, prompt.ID); getErr == nil {
		return a.threadFromChannel(existing, prompt.ID)
	}
	channel, startErr := a.startThreadFromMessage(ctx, prompt.ID, request.Title, request.AutoArchiveMins)
	if startErr != nil {
		if existing, getErr := a.getChannel(ctx, prompt.ID); getErr == nil {
			return a.threadFromChannel(existing, prompt.ID)
		}
		return Thread{}, startErr
	}
	return a.threadFromChannel(channel, prompt.ID)
}

func (a *DiscordAdapter) ThreadMessages(ctx context.Context, request ThreadMessagesRequest) (ThreadTranscript, error) {
	thread, err := a.scopedThread(ctx, request.ThreadID)
	if err != nil {
		return ThreadTranscript{}, err
	}
	allowed := normalizedDiscordUserIDs(request.AllowedUserIDs)
	if len(allowed) == 0 && !a.config.AllowAnyUser {
		return ThreadTranscript{}, errors.New("at least one allowed Discord user id is required unless allow-any-user is explicitly enabled")
	}
	bot, err := a.currentUser(ctx)
	if err != nil {
		return ThreadTranscript{}, err
	}
	values := url.Values{}
	values.Set("limit", fmt.Sprintf("%d", request.Limit))
	if request.AfterMessageID != "" {
		values.Set("after", request.AfterMessageID)
	}
	var messages []discordMessage
	path := "/channels/" + url.PathEscape(request.ThreadID) + "/messages"
	if err := a.doJSON(ctx, http.MethodGet, path, values, nil, &messages); err != nil {
		return ThreadTranscript{}, fmt.Errorf("read Discord thread messages: %w", err)
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return discordSnowflakeLess(messages[i].ID, messages[j].ID)
	})
	transcript := ThreadTranscript{Thread: thread, Messages: []ThreadMessage{}}
	for _, message := range messages {
		if strings.TrimSpace(message.ID) != "" {
			transcript.NextAfter = message.ID
		}
		text := strings.TrimSpace(message.Content)
		if text == "" {
			continue
		}
		role := ""
		switch {
		case message.Author.Bot && message.Author.ID == bot.ID:
			role = "agent"
		case !message.Author.Bot:
			if len(allowed) > 0 {
				if _, ok := allowed[message.Author.ID]; ok {
					role = "human"
				}
			} else if a.config.AllowAnyUser {
				role = "human"
			}
		}
		if role == "" {
			continue
		}
		addressed := messageAddressedByBot(message, request.AddressedEmoji)
		if request.UnaddressedOnly && role == "human" && addressed {
			continue
		}
		transcript.Messages = append(transcript.Messages, ThreadMessage{
			ID:             message.ID,
			ThreadID:       request.ThreadID,
			Text:           text,
			Role:           role,
			AuthorID:       message.Author.ID,
			AuthorUsername: message.Author.Username,
			Timestamp:      message.Timestamp,
			Addressed:      addressed,
		})
	}
	return transcript, nil
}

func (a *DiscordAdapter) ReplyThread(ctx context.Context, request ThreadReplyRequest) (ThreadMessage, error) {
	if _, err := a.scopedThread(ctx, request.ThreadID); err != nil {
		return ThreadMessage{}, err
	}
	bot, err := a.currentUser(ctx)
	if err != nil {
		return ThreadMessage{}, err
	}
	content := formatThreadReply(request)
	marker := discordThreadReplyMarker(request.IdempotencyKey)
	payloadMarker := discordThreadReplyPayloadMarker(request)
	if marker != "" {
		message, found, findErr := a.findBotMessageInChannel(ctx, request.ThreadID, marker, payloadMarker, bot.ID)
		if findErr != nil {
			return ThreadMessage{}, findErr
		}
		if found {
			return threadMessageFromDiscord(message, request.ThreadID, "agent"), nil
		}
	}
	nonce := ""
	if request.IdempotencyKey != "" {
		nonce = discordQuestionNonce(request.ThreadID, "reply:"+request.IdempotencyKey)
	}
	message, err := a.createMessageInChannel(ctx, request.ThreadID, content, nonce)
	if err != nil {
		return ThreadMessage{}, err
	}
	if payloadMarker != "" && !strings.Contains(message.Content, payloadMarker) {
		return ThreadMessage{}, errors.New("thread reply idempotency key resolved to a Discord message with different content")
	}
	return threadMessageFromDiscord(message, request.ThreadID, "agent"), nil
}

func (a *DiscordAdapter) WaitThread(ctx context.Context, request ThreadWaitRequest) (ThreadMessage, error) {
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
	}
	cursor := request.AfterMessageID
	for {
		transcript, err := a.ThreadMessages(ctx, ThreadMessagesRequest{
			ThreadID:       request.ThreadID,
			AfterMessageID: cursor,
			Limit:          100,
			AllowedUserIDs: request.AllowedUserIDs,
			AddressedEmoji: request.AddressedEmoji,
		})
		if err != nil {
			return ThreadMessage{}, err
		}
		for _, message := range transcript.Messages {
			if message.Role == "human" && !message.Addressed {
				return message, nil
			}
		}
		if transcript.NextAfter != "" {
			cursor = transcript.NextAfter
		}
		timer := time.NewTimer(request.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ThreadMessage{}, fmt.Errorf("wait for Discord thread message: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func (a *DiscordAdapter) startThreadFromMessage(ctx context.Context, messageID, title string, autoArchiveMins int) (discordChannel, error) {
	var channel discordChannel
	request := discordStartThreadRequest{Name: title, AutoArchiveDuration: autoArchiveMins}
	path := "/channels/" + url.PathEscape(a.config.ChannelID) + "/messages/" + url.PathEscape(messageID) + "/threads"
	if err := a.doJSON(ctx, http.MethodPost, path, nil, request, &channel); err != nil {
		return discordChannel{}, fmt.Errorf("start Discord thread: %w", err)
	}
	return channel, nil
}

func (a *DiscordAdapter) getChannel(ctx context.Context, channelID string) (discordChannel, error) {
	var channel discordChannel
	if err := a.doJSON(ctx, http.MethodGet, "/channels/"+url.PathEscape(channelID), nil, nil, &channel); err != nil {
		return discordChannel{}, err
	}
	return channel, nil
}

func (a *DiscordAdapter) scopedThread(ctx context.Context, threadID string) (Thread, error) {
	channel, err := a.getChannel(ctx, threadID)
	if err != nil {
		return Thread{}, fmt.Errorf("read Discord thread: %w", err)
	}
	return a.threadFromChannel(channel, channel.ID)
}

func (a *DiscordAdapter) threadFromChannel(channel discordChannel, starterMessageID string) (Thread, error) {
	if !isDiscordThreadType(channel.Type) {
		return Thread{}, errors.New("Discord channel is not a thread")
	}
	expectedParent := a.config.ChannelID
	if a.config.ParentChannelID != "" {
		expectedParent = a.config.ParentChannelID
	}
	if strings.TrimSpace(channel.ParentID) != strings.TrimSpace(expectedParent) {
		return Thread{}, errors.New("Discord thread is outside the configured parent channel")
	}
	return Thread{
		ID:               channel.ID,
		ParentChannelID:  channel.ParentID,
		StarterMessageID: starterMessageID,
		GuildID:          channel.GuildID,
		Name:             channel.Name,
		Archived:         channel.ThreadMetadata.Archived,
	}, nil
}

func isDiscordThreadType(channelType int) bool {
	return channelType == 10 || channelType == 11 || channelType == 12
}

func discordSnowflakeLess(left, right string) bool {
	left = strings.TrimLeft(strings.TrimSpace(left), "0")
	right = strings.TrimLeft(strings.TrimSpace(right), "0")
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return left < right
}

func threadMessageFromDiscord(message discordMessage, threadID, role string) ThreadMessage {
	return ThreadMessage{
		ID:             message.ID,
		ThreadID:       threadID,
		Text:           strings.TrimSpace(message.Content),
		Role:           role,
		AuthorID:       message.Author.ID,
		AuthorUsername: message.Author.Username,
		Timestamp:      message.Timestamp,
		Addressed:      messageAddressedByBot(message, DefaultAddressedEmoji),
	}
}

func (a *DiscordAdapter) AckThread(ctx context.Context, request ThreadAckRequest) error {
	if _, err := a.scopedThread(ctx, request.ThreadID); err != nil {
		return err
	}
	var message discordMessage
	path := "/channels/" + url.PathEscape(request.ThreadID) +
		"/messages/" + url.PathEscape(request.MessageID)
	if err := a.doJSON(ctx, http.MethodGet, path, nil, nil, &message); err != nil {
		return fmt.Errorf("read Discord thread message: %w", err)
	}
	if strings.TrimSpace(message.ChannelID) != "" &&
		strings.TrimSpace(message.ChannelID) != strings.TrimSpace(request.ThreadID) {
		return errors.New("Discord message is outside the thread")
	}
	if err := a.createReactionInChannel(ctx, request.ThreadID, request.MessageID, request.Emoji); err != nil {
		return err
	}
	return nil
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
	return a.createReactionInChannel(ctx, a.config.ChannelID, messageID, emoji)
}

func (a *DiscordAdapter) createReactionInChannel(ctx context.Context, channelID, messageID, emoji string) error {
	path := "/channels/" + url.PathEscape(channelID) +
		"/messages/" + url.PathEscape(messageID) +
		"/reactions/" + discordReactionPathEmoji(emoji) + "/@me"
	if err := a.doJSON(ctx, http.MethodPut, path, nil, nil, nil); err != nil {
		return fmt.Errorf("create discord reaction: %w", err)
	}
	return nil
}

func messageAddressedByBot(message discordMessage, emoji string) bool {
	emoji = normalizeAddressedEmoji(emoji)
	for _, reaction := range message.Reactions {
		if !reaction.Me {
			continue
		}
		if discordEmojiEquals(reaction.Emoji, emoji) {
			return true
		}
	}
	return false
}

func discordEmojiEquals(got discordEmoji, want string) bool {
	want = strings.TrimSpace(want)
	name := strings.TrimSpace(got.Name)
	id := strings.TrimSpace(got.ID)
	if id != "" {
		return want == name+":"+id || want == id
	}
	return want == name
}

func (a *DiscordAdapter) waitForReply(ctx context.Context, questionMessageID string, question Question) (Response, error) {
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
	const maxPages = 100
	before := ""
	var all []discordMessage
	for page := 0; page < maxPages; page++ {
		values := url.Values{}
		values.Set("limit", "100")
		if before != "" {
			values.Set("before", before)
		}
		var messages []discordMessage
		if err := a.doJSON(ctx, http.MethodGet, "/channels/"+url.PathEscape(a.config.ChannelID)+"/messages", values, nil, &messages); err != nil {
			return nil, fmt.Errorf("poll discord replies: %w", err)
		}
		for _, message := range messages {
			if strings.TrimSpace(message.ID) == strings.TrimSpace(messageID) {
				return all, nil
			}
			all = append(all, message)
		}
		if len(messages) < 100 {
			return all, nil
		}
		before = strings.TrimSpace(messages[len(messages)-1].ID)
		if before == "" {
			return nil, errors.New("discord reply page did not include an oldest message id")
		}
	}
	return nil, errors.New("discord reply history exceeded pagination limit")
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
	Content         string                 `json:"content"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
	Nonce           string                 `json:"nonce,omitempty"`
	EnforceNonce    bool                   `json:"enforce_nonce,omitempty"`
}

type discordAllowedMentions struct {
	Parse []string `json:"parse"`
}

type discordMessage struct {
	ID               string                   `json:"id"`
	ChannelID        string                   `json:"channel_id"`
	Content          string                   `json:"content"`
	Author           discordUser              `json:"author"`
	Timestamp        string                   `json:"timestamp"`
	MessageReference *discordMessageReference `json:"message_reference"`
	Thread           *discordChannel          `json:"thread"`
	Reactions        []discordReaction        `json:"reactions"`
}

type discordReaction struct {
	Count int          `json:"count"`
	Me    bool         `json:"me"`
	Emoji discordEmoji `json:"emoji"`
}

type discordEmoji struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type discordChannel struct {
	ID             string                `json:"id"`
	GuildID        string                `json:"guild_id"`
	ParentID       string                `json:"parent_id"`
	Name           string                `json:"name"`
	Type           int                   `json:"type"`
	ThreadMetadata discordThreadMetadata `json:"thread_metadata"`
}

type discordThreadMetadata struct {
	Archived bool `json:"archived"`
}

type discordStartThreadRequest struct {
	Name                string `json:"name"`
	AutoArchiveDuration int    `json:"auto_archive_duration,omitempty"`
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
