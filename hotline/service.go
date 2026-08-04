// Package hotline is the transport-agnostic agent feedback library used by
// Anvil agents when they need a bounded human reply before continuing.
// Discord is the first transport: post one question, poll for an authorized
// reply, and return only the reply text.
package hotline

import (
	"context"
	"errors"
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

// Question is one human escalation. Prompt is required; Context and RunName
// are optional message enrichment. AllowedUserIDs is the fail-closed allowlist
// for Discord replies unless the transport explicitly allows any non-bot user.
// Reactions, when set, are pre-applied to the question so the human can click
// a choice instead of typing; typed replies remain accepted.
type Question struct {
	Prompt         string
	Context        string
	RunName        string
	Timeout        time.Duration
	PollInterval   time.Duration
	AllowedUserIDs []string
	AcceptAnyAfter bool
	Reactions      []ReactionOption
}

// Response is the accepted human reply after transport filtering.
// Source is "reply" for a typed message or "reaction" for an emoji choice.
type Response struct {
	Text           string `json:"text"`
	Source         string `json:"source,omitempty"`
	AuthorID       string `json:"authorId,omitempty"`
	AuthorUsername string `json:"authorUsername,omitempty"`
	MessageID      string `json:"messageId,omitempty"`
	ChannelID      string `json:"channelId,omitempty"`
	ReactionEmoji  string `json:"reactionEmoji,omitempty"`
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
	return s.Transport.Ask(ctx, question)
}

// FormatQuestionMessage builds the Discord (or other chat) payload.
func FormatQuestionMessage(question Question) string {
	var builder strings.Builder
	builder.WriteString("**Anvil Hotline — agent needs a reply**\n\n")
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
		builder.WriteString("\nOr reply directly to this message. The agent is waiting for an authorized choice.")
	} else {
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
