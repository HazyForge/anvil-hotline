package hotline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscordAdapterWaitsForReplyToQuestion(t *testing.T) {
	t.Parallel()

	var polls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bot test-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/channels/c123/messages":
			var payload discordCreateMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(payload.Content, "Should I restart the controller?") {
				t.Fatalf("posted content = %q, want question", payload.Content)
			}
			if !strings.Contains(payload.Content, "Anvil Hotline") {
				t.Fatalf("posted content = %q, want Anvil Hotline branding", payload.Content)
			}
			if len(payload.AllowedMentions.Parse) != 0 {
				t.Fatalf("allowed mentions parse = %#v, want empty", payload.AllowedMentions.Parse)
			}
			_, _ = w.Write([]byte(`{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			if got, want := r.URL.Query().Get("after"), "100"; got != want {
				t.Fatalf("after = %q, want %q", got, want)
			}
			if atomic.AddInt32(&polls, 1) == 1 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"id":"102","channel_id":"c123","content":"ignore non reply","author":{"id":"u1","username":"Austin","bot":false}},
				{"id":"101","channel_id":"c123","content":"yes, restart it","author":{"id":"u1","username":"Austin","bot":false},"message_reference":{"message_id":"100","channel_id":"c123"}}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:   "test-token",
		ChannelID:  "c123",
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := adapter.Ask(context.Background(), Question{
		Prompt:         "Should I restart the controller?",
		PollInterval:   time.Millisecond,
		Timeout:        time.Second,
		AllowedUserIDs: []string{"u1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := response.Text, "yes, restart it"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	if got, want := response.AuthorID, "u1"; got != want {
		t.Fatalf("author id = %q, want %q", got, want)
	}
}

func TestDiscordAdapterCanAcceptAnyMessageAfterQuestion(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":"101","channel_id":"c123","content":"continue","author":{"id":"u1","username":"Austin","bot":false}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:     "test-token",
		ChannelID:    "c123",
		APIBaseURL:   server.URL,
		HTTPClient:   server.Client(),
		AllowAnyUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := adapter.Ask(context.Background(), Question{
		Prompt:         "Continue?",
		PollInterval:   time.Millisecond,
		Timeout:        time.Second,
		AcceptAnyAfter: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := response.Text, "continue"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
}

func TestDiscordAdapterRequiresAuthorizedUserByDefault(t *testing.T) {
	t.Parallel()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:  "test-token",
		ChannelID: "c123",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Ask(context.Background(), Question{Prompt: "Continue?"})
	if err == nil || !strings.Contains(err.Error(), "allowed Discord user id") {
		t.Fatalf("Ask() error = %v, want missing allowlist error", err)
	}
}

func TestDiscordAdapterAcceptsAuthorizedReaction(t *testing.T) {
	t.Parallel()

	var applied []string
	var polls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bot test-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/channels/c123/messages":
			var payload discordCreateMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(payload.Content, "✅ → `yes`") {
				t.Fatalf("posted content = %q, want reaction choices", payload.Content)
			}
			if !strings.Contains(payload.Content, "Choose a reaction") {
				t.Fatalf("posted content = %q, want choose instruction", payload.Content)
			}
			_, _ = w.Write([]byte(`{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/channels/c123/messages/100/reactions/"):
			// Path is /reactions/{emoji}/@me
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) < 2 || parts[len(parts)-1] != "@me" {
				t.Fatalf("unexpected reaction path %q", r.URL.Path)
			}
			applied = append(applied, parts[len(parts)-2])
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/channels/c123/messages/100/reactions/"):
			emoji := strings.TrimPrefix(r.URL.Path, "/channels/c123/messages/100/reactions/")
			if atomic.AddInt32(&polls, 1) <= 2 {
				// First pass: only the bot's pre-applied reaction.
				_, _ = w.Write([]byte(`[{"id":"bot","username":"hotline","bot":true}]`))
				return
			}
			if emoji == "%E2%9C%85" || emoji == "✅" { // ✅ may be path-escaped by client
				_, _ = w.Write([]byte(`[
					{"id":"bot","username":"hotline","bot":true},
					{"id":"u1","username":"Austin","bot":false}
				]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"bot","username":"hotline","bot":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:   "test-token",
		ChannelID:  "c123",
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := adapter.Ask(context.Background(), Question{
		Prompt:         "May I proceed with the proposed default?",
		PollInterval:   time.Millisecond,
		Timeout:        time.Second,
		AllowedUserIDs: []string{"u1"},
		Reactions: []ReactionOption{
			{Emoji: "✅", Value: "yes"},
			{Emoji: "❌", Value: "no"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := response.Text, "yes"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	if got, want := response.Source, "reaction"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if got, want := response.ReactionEmoji, "✅"; got != want {
		t.Fatalf("reaction emoji = %q, want %q", got, want)
	}
	if got, want := response.AuthorID, "u1"; got != want {
		t.Fatalf("author id = %q, want %q", got, want)
	}
	if len(applied) != 2 {
		t.Fatalf("applied reactions = %#v, want 2", applied)
	}
}

func TestDiscordAdapterStillAcceptsTypedReplyWhenReactionsConfigured(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}`))
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_, _ = w.Write([]byte(`[
				{"id":"101","channel_id":"c123","content":"typed yes please","author":{"id":"u1","username":"Austin","bot":false},"message_reference":{"message_id":"100","channel_id":"c123"}}
			]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reactions/"):
			_, _ = w.Write([]byte(`[{"id":"bot","username":"hotline","bot":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:   "test-token",
		ChannelID:  "c123",
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := adapter.Ask(context.Background(), Question{
		Prompt:         "Proceed?",
		PollInterval:   time.Millisecond,
		Timeout:        time.Second,
		AllowedUserIDs: []string{"u1"},
		Reactions: []ReactionOption{
			{Emoji: "✅", Value: "yes"},
			{Emoji: "❌", Value: "no"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := response.Text, "typed yes please"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	if got, want := response.Source, "reply"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
}

func TestDiscordAdapterIgnoresUnauthorizedReaction(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}`))
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reactions/"):
			_, _ = w.Write([]byte(`[{"id":"stranger","username":"Nope","bot":false}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:   "test-token",
		ChannelID:  "c123",
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Ask(context.Background(), Question{
		Prompt:         "Proceed?",
		PollInterval:   time.Millisecond,
		Timeout:        30 * time.Millisecond,
		AllowedUserIDs: []string{"u1"},
		Reactions:      []ReactionOption{{Emoji: "✅", Value: "yes"}},
	})
	if err == nil {
		t.Fatal("Ask() succeeded for unauthorized reaction, want timeout")
	}
	// Timeout may surface while polling messages or while waiting between polls.
	if !strings.Contains(err.Error(), "context deadline exceeded") &&
		!strings.Contains(err.Error(), "wait for discord reply") {
		t.Fatalf("Ask() error = %v, want timeout while waiting for authorized reaction", err)
	}
}

func TestDiscordAdapterPostsMultipartAttachments(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	imagePath := filepath.Join(tmp, "design.png")
	// Minimal PNG header + payload is enough for upload plumbing tests.
	imageBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	}
	if err := os.WriteFile(imagePath, imageBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	var sawMultipart bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/channels/c123/messages":
			contentType := r.Header.Get("Content-Type")
			if !strings.HasPrefix(contentType, "multipart/form-data") {
				t.Fatalf("content-type = %q, want multipart/form-data", contentType)
			}
			reader, err := r.MultipartReader()
			if err != nil {
				t.Fatal(err)
			}
			var payloadJSON string
			var fileName string
			var fileData []byte
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(part)
				if err != nil {
					t.Fatal(err)
				}
				switch part.FormName() {
				case "payload_json":
					payloadJSON = string(data)
				case "files[0]":
					fileName = part.FileName()
					fileData = data
				}
			}
			if !strings.Contains(payloadJSON, "Like this layout?") {
				t.Fatalf("payload_json = %q, want question", payloadJSON)
			}
			if !strings.Contains(payloadJSON, `"filename":"design.png"`) {
				t.Fatalf("payload_json = %q, want attachment metadata", payloadJSON)
			}
			if fileName != "design.png" {
				t.Fatalf("file name = %q, want design.png", fileName)
			}
			if !bytes.Equal(fileData, imageBytes) {
				t.Fatalf("file bytes mismatch: got %d want %d", len(fileData), len(imageBytes))
			}
			sawMultipart = true
			_, _ = w.Write([]byte(`{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}`))
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reactions/"):
			_, _ = w.Write([]byte(`[
				{"id":"bot","username":"hotline","bot":true},
				{"id":"u1","username":"Austin","bot":false}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:   "test-token",
		ChannelID:  "c123",
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	response, err := adapter.Ask(context.Background(), Question{
		Prompt:         "Like this layout?",
		PollInterval:   time.Millisecond,
		Timeout:        time.Second,
		AllowedUserIDs: []string{"u1"},
		Reactions:      []ReactionOption{{Emoji: "✅", Value: "yes"}, {Emoji: "❌", Value: "no"}},
		Attachments:    []Attachment{{Path: imagePath}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawMultipart {
		t.Fatal("expected multipart upload")
	}
	if got, want := response.Text, "yes"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
}

func TestNormalizeAttachmentsRejectsTooMany(t *testing.T) {
	t.Parallel()

	attachments := make([]Attachment, MaxAttachments+1)
	for i := range attachments {
		attachments[i] = Attachment{Path: fmt.Sprintf("/tmp/file-%d.png", i)}
	}
	_, err := normalizeAttachments(attachments)
	if err == nil || !strings.Contains(err.Error(), "too many attachments") {
		t.Fatalf("error = %v, want too many attachments", err)
	}
}

func TestParseReactionOption(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw       string
		wantEmoji string
		wantValue string
	}{
		{raw: "✅=yes", wantEmoji: "✅", wantValue: "yes"},
		{raw: "❌:no", wantEmoji: "❌", wantValue: "no"},
		{raw: "👍", wantEmoji: "👍", wantValue: "👍"},
	}
	for _, tc := range cases {
		got, err := ParseReactionOption(tc.raw)
		if err != nil {
			t.Fatalf("ParseReactionOption(%q) error: %v", tc.raw, err)
		}
		if got.Emoji != tc.wantEmoji || got.Value != tc.wantValue {
			t.Fatalf("ParseReactionOption(%q) = %+v, want emoji=%q value=%q", tc.raw, got, tc.wantEmoji, tc.wantValue)
		}
	}
}

func TestFormatQuestionMessageIncludesReactions(t *testing.T) {
	t.Parallel()

	message := FormatQuestionMessage(Question{
		Prompt: "Ship it?",
		Reactions: []ReactionOption{
			{Emoji: "✅", Value: "yes"},
			{Emoji: "❌", Value: "no"},
		},
	})
	for _, want := range []string{"Ship it?", "Choose a reaction", "✅ → `yes`", "❌ → `no`", "Or reply directly"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
}

func TestFormatQuestionMessageDesignIsReplyBased(t *testing.T) {
	t.Parallel()

	message := FormatQuestionMessage(Question{
		Prompt:        "Like this layout?",
		FeedbackStyle: "design",
	})
	for _, want := range []string{
		"design review",
		"Reply",
		"free-text feedback",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("design message missing %q:\n%s", want, message)
		}
	}
}
