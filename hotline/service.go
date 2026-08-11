// Package hotline is the transport-agnostic agent feedback library used by
// Anvil agents when they need a bounded human reply before continuing.
// Discord is the first transport: post one question, poll for an authorized
// reply, and return only the reply text.
package hotline

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"
)

var ErrUnsupportedTransport = errors.New("unsupported feedback transport")

// ReactionOption is one pre-applied emoji choice on the question message.
// When an authorized human reacts with Emoji, Ask returns Value (or Emoji
// when Value is empty) instead of requiring a typed reply.
type ReactionOption struct {
	Emoji string
	Value string
}

// Attachment is a local file to upload with the question (for example a design
// mockup PNG). Path is required; Filename and ContentType are optional and
// default from the path when empty.
type Attachment struct {
	Path        string
	Filename    string
	ContentType string
}

// Question is one human escalation. Prompt is required; Context and RunName
// are optional message enrichment. AllowedUserIDs is the fail-closed allowlist
// for Discord replies unless the transport explicitly allows any non-bot user.
// Reactions, when set, are pre-applied to the question so the human can click
// a choice instead of typing; typed replies remain accepted.
// Attachments, when set, are uploaded as Discord message files so the human
// can see images (or other files) while choosing a reaction or typing a reply.
//
// Feedback model (keep simple):
//   - A typed reply with no reaction is a complete answer (full critique in Text).
//   - A reaction with no typed reply is a complete answer (choice in Text).
//   - CollectNotes is optional only: after a reaction, wait briefly for extra
//     free-text if the human wants to add likes/dislikes. Never required.
//   - Do not force notes after any reaction.
type Question struct {
	Prompt         string
	Context        string
	RunName        string
	Timeout        time.Duration
	PollInterval   time.Duration
	AllowedUserIDs []string
	AcceptAnyAfter bool
	Reactions      []ReactionOption
	Attachments    []Attachment
	// CollectNotes, when true, waits NotesTimeout after a reaction for optional
	// free-text. A reaction alone still completes if no notes arrive.
	CollectNotes bool
	NotesTimeout time.Duration
	// FeedbackStyle controls the human-facing instructions in the Discord
	// message. Empty/"default" keeps the classic ask wording. "design"
	// treats react and free-text reply as equal complete answers.
	FeedbackStyle string
}

// Response is the accepted human reply after transport filtering.
// Source is "reply" for a typed message, "reaction" for an emoji choice, or
// "reaction+notes" when a reaction was chosen and optional free-text followed.
type Response struct {
	Text           string `json:"text"`
	Notes          string `json:"notes,omitempty"`
	Source         string `json:"source,omitempty"`
	AuthorID       string `json:"authorId,omitempty"`
	AuthorUsername string `json:"authorUsername,omitempty"`
	MessageID      string `json:"messageId,omitempty"`
	ChannelID      string `json:"channelId,omitempty"`
	ReactionEmoji  string `json:"reactionEmoji,omitempty"`
	// Choice is set for reactions (the mapped value). Free-text-only replies
	// leave Choice empty; Text is the full answer.
	Choice string `json:"choice,omitempty"`
}

// Transport posts a question and waits for an authorized reply.
type Transport interface {
	Ask(ctx context.Context, question Question) (Response, error)
}

// Service validates a Question and delegates to Transport.
type Service struct {
	Transport Transport
}

func (s Service) Ask(ctx context.Context, question Question) (Response, error) {
	if s.Transport == nil {
		return Response{}, ErrUnsupportedTransport
	}
	question.Prompt = strings.TrimSpace(question.Prompt)
	question.Context = strings.TrimSpace(question.Context)
	question.RunName = strings.TrimSpace(question.RunName)
	if question.Prompt == "" {
		return Response{}, errors.New("question prompt is required")
	}
	if question.PollInterval <= 0 {
		question.PollInterval = 5 * time.Second
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
	question.FeedbackStyle = normalizeFeedbackStyle(question.FeedbackStyle)
	if question.CollectNotes && question.NotesTimeout <= 0 {
		question.NotesTimeout = 45 * time.Second
	}
	return s.Transport.Ask(ctx, question)
}

func normalizeFeedbackStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", "default":
		return "default"
	case "design", "design-review", "mockup":
		return "design"
	default:
		return strings.ToLower(strings.TrimSpace(style))
	}
}

// DesignReviewReactions builds a common design-review reaction set for N
// mockup variants (1–9), plus approve / revise / reject.
// Numbered picks map to design-1 … design-N. Free-text replies are always a
// complete answer without needing these reactions.
func DesignReviewReactions(variantCount int) ([]ReactionOption, error) {
	if variantCount < 0 {
		return nil, errors.New("design review variant count must be >= 0")
	}
	if variantCount > 9 {
		return nil, errors.New("design review supports at most 9 numbered variants")
	}
	// keycap digits 1-9
	keycaps := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}
	out := make([]ReactionOption, 0, variantCount+3)
	for i := 0; i < variantCount; i++ {
		out = append(out, ReactionOption{
			Emoji: keycaps[i],
			Value: fmt.Sprintf("design-%d", i+1),
		})
	}
	out = append(out,
		ReactionOption{Emoji: "✅", Value: "approve"},
		ReactionOption{Emoji: "🔄", Value: "revise"},
		ReactionOption{Emoji: "❌", Value: "reject"},
	)
	return normalizeReactionOptions(out)
}

func shouldCollectNotes(question Question, response Response) bool {
	// Optional only — never force typed follow-up after a reaction.
	return question.CollectNotes && response.Source == "reaction"
}

// MaxAttachments is the Discord multi-file limit for one message.
const MaxAttachments = 10

// MaxAttachmentBytes is a conservative per-file cap for non-boosted bots.
const MaxAttachmentBytes = 25 * 1024 * 1024

func normalizeAttachments(attachments []Attachment) ([]Attachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	if len(attachments) > MaxAttachments {
		return nil, fmt.Errorf("too many attachments: %d (max %d)", len(attachments), MaxAttachments)
	}
	out := make([]Attachment, 0, len(attachments))
	for i, attachment := range attachments {
		path := strings.TrimSpace(attachment.Path)
		if path == "" {
			return nil, fmt.Errorf("attachment %d path is required", i)
		}
		filename := strings.TrimSpace(attachment.Filename)
		if filename == "" {
			filename = filepath.Base(path)
		}
		if filename == "" || filename == "." || filename == string(filepath.Separator) {
			return nil, fmt.Errorf("attachment %d filename is required", i)
		}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = contentTypeFromFilename(filename)
		}
		out = append(out, Attachment{
			Path:        path,
			Filename:    filename,
			ContentType: contentType,
		})
	}
	return out, nil
}

func contentTypeFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".txt", ".md", ".log":
		return "text/plain"
	default:
		if mime := mime.TypeByExtension(ext); mime != "" {
			return mime
		}
		return "application/octet-stream"
	}
}

// FormatQuestionMessage builds the Discord (or other chat) payload.
func FormatQuestionMessage(question Question) string {
	var builder strings.Builder
	style := normalizeFeedbackStyle(question.FeedbackStyle)
	if style == "design" {
		builder.WriteString("**Anvil Hotline — design review**\n\n")
	} else {
		builder.WriteString("**Anvil Hotline — agent needs a reply**\n\n")
	}
	if question.RunName != "" {
		builder.WriteString("AgentRun: `")
		builder.WriteString(question.RunName)
		builder.WriteString("`\n\n")
	}
	builder.WriteString("Question:\n")
	builder.WriteString(question.Prompt)
	if question.Context != "" {
		builder.WriteString("\n\nContext:\n")
		builder.WriteString(question.Context)
	}
	if len(question.Reactions) > 0 {
		builder.WriteString("\n\nChoose a reaction")
		if len(question.Reactions) > 1 {
			builder.WriteString(" (click one)")
		}
		builder.WriteString(":\n")
		for _, reaction := range question.Reactions {
			builder.WriteString(reaction.Emoji)
			builder.WriteString(" → `")
			builder.WriteString(reaction.Value)
			builder.WriteString("`\n")
		}
	}

	switch {
	case style == "design":
		builder.WriteString("\n**How to answer (either is enough)**\n")
		builder.WriteString("• **Reply** with free-text: what you like vs what you don't — full answer, no reaction needed\n")
		if len(question.Reactions) > 0 {
			builder.WriteString("• **Or react** to pick a design / path — reaction alone is enough\n")
		}
		builder.WriteString("\nReply directly to this message when typing. The agent is waiting.")
	case len(question.Reactions) > 0:
		builder.WriteString("\nOr reply directly to this message (typed reply alone is a complete answer). The agent is waiting.")
	default:
		builder.WriteString("\n\nPlease reply directly to this message. The agent is waiting for an authorized reply.")
	}
	return limitRunes(builder.String(), 1900)
}

// ParseReactionOption parses "emoji=value" (preferred) or "emoji:value".
// When no separator is present, the emoji is both the reaction and the returned
// value. Colon form is skipped when the right side looks like a Discord custom
// emoji id (all digits), so "thumbsup:1234567890" stays a single emoji token.
func ParseReactionOption(raw string) (ReactionOption, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ReactionOption{}, errors.New("reaction option is empty")
	}
	emoji, value, ok := strings.Cut(raw, "=")
	if !ok {
		left, right, cut := strings.Cut(raw, ":")
		if cut && right != "" && !isAllDigits(right) {
			emoji, value = left, right
		} else {
			emoji, value = raw, ""
		}
	}
	option := ReactionOption{
		Emoji: strings.TrimSpace(emoji),
		Value: strings.TrimSpace(value),
	}
	normalized, err := normalizeReactionOptions([]ReactionOption{option})
	if err != nil {
		return ReactionOption{}, err
	}
	return normalized[0], nil
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeReactionOptions(options []ReactionOption) ([]ReactionOption, error) {
	if len(options) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(options))
	out := make([]ReactionOption, 0, len(options))
	for _, option := range options {
		emoji := strings.TrimSpace(option.Emoji)
		value := strings.TrimSpace(option.Value)
		if emoji == "" {
			return nil, errors.New("reaction emoji is required")
		}
		if value == "" {
			value = emoji
		}
		if _, exists := seen[emoji]; exists {
			return nil, errors.New("duplicate reaction emoji: " + emoji)
		}
		seen[emoji] = struct{}{}
		out = append(out, ReactionOption{Emoji: emoji, Value: value})
	}
	return out, nil
}

func limitRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	suffix := "\n\n[truncated by anvil-hotline; ask a narrower follow-up if needed]"
	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= limit {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-len(suffixRunes)])) + suffix
}
