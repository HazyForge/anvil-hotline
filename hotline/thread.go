package hotline

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrThreadsUnsupported = errors.New("hotline transport does not support threads")

// Thread is one durable, parent-channel-scoped Hotline conversation.
type Thread struct {
	ID               string `json:"id"`
	ParentChannelID  string `json:"parentChannelId"`
	StarterMessageID string `json:"starterMessageId"`
	GuildID          string `json:"guildId,omitempty"`
	Name             string `json:"name"`
	Archived         bool   `json:"archived"`
}

// DefaultAddressedEmoji is the Discord reaction the bot applies to a human
// thread message after the agent has processed it. Later reads and waits skip
// those messages so the same reply is not handled twice.
const DefaultAddressedEmoji = "✅"

// ThreadMessage is one trusted conversation message. Role is "human" for an
// allowed non-bot user and "agent" for the currently authenticated Hotline bot.
// Addressed is true when this bot has already reacted with the addressed emoji.
type ThreadMessage struct {
	ID             string `json:"id"`
	ThreadID       string `json:"threadId"`
	Text           string `json:"text"`
	Role           string `json:"role"`
	AuthorID       string `json:"authorId"`
	AuthorUsername string `json:"authorUsername,omitempty"`
	Timestamp      string `json:"timestamp,omitempty"`
	Addressed      bool   `json:"addressed"`
}

// ThreadTranscript is a chronological, trusted subset of one thread. Callers
// can pass NextAfter to a later read or wait without replaying older messages.
type ThreadTranscript struct {
	Thread    Thread          `json:"thread"`
	Messages  []ThreadMessage `json:"messages"`
	NextAfter string          `json:"nextAfter,omitempty"`
}

// ThreadRequest opens or resumes a discussion thread from one stable starter
// message in the configured parent channel.
type ThreadRequest struct {
	Title           string
	Message         string
	Context         string
	RunName         string
	IdempotencyKey  string
	AutoArchiveMins int
	AllowedUserIDs  []string
}

// ThreadMessagesRequest reads trusted messages from one child thread.
type ThreadMessagesRequest struct {
	ThreadID       string
	AfterMessageID string
	Limit          int
	AllowedUserIDs []string
	// AddressedEmoji is the bot reaction that marks a human message as already
	// processed. Empty uses DefaultAddressedEmoji.
	AddressedEmoji string
	// UnaddressedOnly omits human messages the bot has already addressed.
	UnaddressedOnly bool
}

// ThreadReplyRequest posts or resumes one bot-authored reply in a thread.
type ThreadReplyRequest struct {
	ThreadID       string
	Message        string
	IdempotencyKey string
}

// ThreadWaitRequest waits for the next allowed human message after a cursor.
type ThreadWaitRequest struct {
	ThreadID       string
	AfterMessageID string
	Timeout        time.Duration
	PollInterval   time.Duration
	AllowedUserIDs []string
	// AddressedEmoji is the bot reaction that marks a human message as already
	// processed. Wait skips those messages. Empty uses DefaultAddressedEmoji.
	AddressedEmoji string
}

// ThreadAckRequest marks one thread message as processed by applying the bot's
// addressed reaction. Later waits and unaddressed reads skip it.
type ThreadAckRequest struct {
	ThreadID  string
	MessageID string
	Emoji     string
}

// ThreadTransport adds bounded, parent-scoped conversations to a transport.
type ThreadTransport interface {
	OpenThread(ctx context.Context, request ThreadRequest) (Thread, error)
	ThreadMessages(ctx context.Context, request ThreadMessagesRequest) (ThreadTranscript, error)
	ReplyThread(ctx context.Context, request ThreadReplyRequest) (ThreadMessage, error)
	WaitThread(ctx context.Context, request ThreadWaitRequest) (ThreadMessage, error)
	AckThread(ctx context.Context, request ThreadAckRequest) error
}

func (s Service) threadTransport() (ThreadTransport, error) {
	transport, ok := s.Transport.(ThreadTransport)
	if !ok || transport == nil {
		return nil, ErrThreadsUnsupported
	}
	return transport, nil
}

func (s Service) OpenThread(ctx context.Context, request ThreadRequest) (Thread, error) {
	transport, err := s.threadTransport()
	if err != nil {
		return Thread{}, err
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Message = strings.TrimSpace(request.Message)
	request.Context = strings.TrimSpace(request.Context)
	request.RunName = strings.TrimSpace(request.RunName)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.Title == "" {
		return Thread{}, errors.New("thread title is required")
	}
	if len([]rune(request.Title)) > 100 {
		return Thread{}, errors.New("thread title exceeds Discord's 100-character limit")
	}
	if request.Message == "" {
		return Thread{}, errors.New("thread message is required")
	}
	if request.IdempotencyKey == "" {
		return Thread{}, errors.New("thread idempotency key is required")
	}
	if request.AutoArchiveMins == 0 {
		request.AutoArchiveMins = 1440
	}
	switch request.AutoArchiveMins {
	case 60, 1440, 4320, 10080:
	default:
		return Thread{}, errors.New("thread auto-archive minutes must be one of 60, 1440, 4320, or 10080")
	}
	if len([]rune(FormatThreadStarterMessage(request))) > 2000 {
		return Thread{}, errors.New("thread starter exceeds Discord's 2000-character limit; move detail into keyed thread replies")
	}
	return transport.OpenThread(ctx, request)
}

func (s Service) ThreadMessages(ctx context.Context, request ThreadMessagesRequest) (ThreadTranscript, error) {
	transport, err := s.threadTransport()
	if err != nil {
		return ThreadTranscript{}, err
	}
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.AfterMessageID = strings.TrimSpace(request.AfterMessageID)
	if request.ThreadID == "" {
		return ThreadTranscript{}, errors.New("thread id is required")
	}
	if request.Limit == 0 {
		request.Limit = 100
	}
	if request.Limit < 1 || request.Limit > 100 {
		return ThreadTranscript{}, errors.New("thread message limit must be between 1 and 100")
	}
	request.AddressedEmoji = normalizeAddressedEmoji(request.AddressedEmoji)
	return transport.ThreadMessages(ctx, request)
}

func (s Service) ReplyThread(ctx context.Context, request ThreadReplyRequest) (ThreadMessage, error) {
	transport, err := s.threadTransport()
	if err != nil {
		return ThreadMessage{}, err
	}
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.Message = strings.TrimSpace(request.Message)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ThreadID == "" {
		return ThreadMessage{}, errors.New("thread id is required")
	}
	if request.Message == "" {
		return ThreadMessage{}, errors.New("thread reply message is required")
	}
	if len([]rune(formatThreadReply(request))) > 2000 {
		return ThreadMessage{}, errors.New("thread reply exceeds Discord's 2000-character limit; split it into multiple replies")
	}
	return transport.ReplyThread(ctx, request)
}

func (s Service) WaitThread(ctx context.Context, request ThreadWaitRequest) (ThreadMessage, error) {
	transport, err := s.threadTransport()
	if err != nil {
		return ThreadMessage{}, err
	}
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.AfterMessageID = strings.TrimSpace(request.AfterMessageID)
	if request.ThreadID == "" {
		return ThreadMessage{}, errors.New("thread id is required")
	}
	if request.Timeout < 0 {
		return ThreadMessage{}, errors.New("thread wait timeout cannot be negative")
	}
	if request.PollInterval <= 0 {
		request.PollInterval = 5 * time.Second
	}
	request.AddressedEmoji = normalizeAddressedEmoji(request.AddressedEmoji)
	return transport.WaitThread(ctx, request)
}

func (s Service) AckThread(ctx context.Context, request ThreadAckRequest) error {
	transport, err := s.threadTransport()
	if err != nil {
		return err
	}
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.MessageID = strings.TrimSpace(request.MessageID)
	if request.ThreadID == "" {
		return errors.New("thread id is required")
	}
	if request.MessageID == "" {
		return errors.New("message id is required")
	}
	request.Emoji = normalizeAddressedEmoji(request.Emoji)
	return transport.AckThread(ctx, request)
}

func normalizeAddressedEmoji(emoji string) string {
	emoji = strings.TrimSpace(emoji)
	if emoji == "" {
		return DefaultAddressedEmoji
	}
	return emoji
}

func discordThreadMarker(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return "[anvil-hotline-thread-key:" + fmt.Sprintf("%x", sum[:])[:32] + "]"
}

func discordThreadPayloadMarker(request ThreadRequest) string {
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return ""
	}
	canonical := strings.Join([]string{
		strings.TrimSpace(request.Title),
		strings.TrimSpace(request.Message),
		fmt.Sprintf("%d", request.AutoArchiveMins),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return "[anvil-hotline-thread-payload:" + fmt.Sprintf("%x", sum[:])[:32] + "]"
}

func discordThreadReplyMarker(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return "[anvil-hotline-reply-key:" + fmt.Sprintf("%x", sum[:])[:32] + "]"
}

func discordThreadReplyPayloadMarker(request ThreadReplyRequest) string {
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(request.Message)))
	return "[anvil-hotline-reply-payload:" + fmt.Sprintf("%x", sum[:])[:32] + "]"
}

// FormatThreadStarterMessage builds the parent-channel message from which the
// Discord thread is created. Discussion alone never represents approval.
func FormatThreadStarterMessage(request ThreadRequest) string {
	var builder strings.Builder
	builder.WriteString(discordThreadMarker(request.IdempotencyKey))
	builder.WriteString("\n")
	builder.WriteString(discordThreadPayloadMarker(request))
	builder.WriteString("\n**Anvil Hotline — decision thread**\n\n")
	if request.RunName != "" {
		builder.WriteString("AgentRun: `")
		builder.WriteString(request.RunName)
		builder.WriteString("`\n\n")
	}
	builder.WriteString("Topic:\n")
	builder.WriteString(request.Message)
	if request.Context != "" {
		builder.WriteString("\n\nContext:\n")
		builder.WriteString(request.Context)
	}
	builder.WriteString("\n\nContinue in the attached thread. Discussion is information only; final approval must be explicit and bound to the exact proposed change.")
	return builder.String()
}

func formatThreadReply(request ThreadReplyRequest) string {
	if request.IdempotencyKey == "" {
		return request.Message
	}
	return discordThreadReplyMarker(request.IdempotencyKey) + "\n" +
		discordThreadReplyPayloadMarker(request) + "\n" + request.Message
}
