package hotline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscordAdapterWaitsForReplyToQuestion(t *testing.T) {
	t.Parallel()

	var historyCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bot test-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
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
			if payload.Nonce == "" || !payload.EnforceNonce {
				t.Fatalf("nonce=%q enforce_nonce=%v, want enforced idempotency", payload.Nonce, payload.EnforceNonce)
			}
			_ = json.NewEncoder(w).Encode(discordMessage{ID: "100", ChannelID: "c123", Content: payload.Content, Author: discordUser{ID: "bot", Bot: true}})
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			call := atomic.AddInt32(&historyCalls, 1)
			if call <= 2 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"id":"102","channel_id":"c123","content":"ignore non reply","author":{"id":"u1","username":"Austin","bot":false}},
				{"id":"101","channel_id":"c123","content":"yes, restart it","author":{"id":"u1","username":"Austin","bot":false},"message_reference":{"message_id":"100","channel_id":"c123"}},
				{"id":"100","channel_id":"c123","content":"question","author":{"id":"bot","bot":true}}
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
		IdempotencyKey: "application=release-lab;decision=adopt-staging",
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

func TestDiscordQuestionNonceIsStableAndBounded(t *testing.T) {
	t.Parallel()

	first := discordQuestionNonce("c123", " application=release-lab;decision=adopt-staging ")
	second := discordQuestionNonce("c123", "application=release-lab;decision=adopt-staging")
	if first != second {
		t.Fatalf("nonce is not stable: %q != %q", first, second)
	}
	if len(first) != 25 {
		t.Fatalf("nonce length = %d, want 25", len(first))
	}
	if got := discordQuestionNonce("c123", "  "); got != "" {
		t.Fatalf("empty nonce = %q, want empty", got)
	}
	if other := discordQuestionNonce("c456", "application=release-lab;decision=adopt-staging"); other == first {
		t.Fatalf("nonce should be channel scoped: %q", other)
	}
}

func TestDiscordAdapterResumesExistingIdempotentQuestion(t *testing.T) {
	t.Parallel()

	key := "application=release-lab;decision=adopt-staging"
	storedQuestion := Question{Prompt: "Which target?", Context: "AgentRun=old-run", IdempotencyKey: key}
	question := Question{Prompt: "Which target?", Context: "AgentRun=new-run", IdempotencyKey: key, AllowedUserIDs: []string{"u1"}, Timeout: time.Second, PollInterval: time.Millisecond}
	content := FormatQuestionMessage(storedQuestion)
	var posts int32
	var historyCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			if atomic.AddInt32(&historyCalls, 1) == 1 {
				_ = json.NewEncoder(w).Encode([]discordMessage{{ID: "100", ChannelID: "c123", Content: content, Author: discordUser{ID: "bot", Bot: true}}})
				return
			}
			_ = json.NewEncoder(w).Encode([]discordMessage{{ID: "101", ChannelID: "c123", Content: "use staging", Author: discordUser{ID: "u1"}, MessageReference: &discordMessageReference{MessageID: "100", ChannelID: "c123"}}, {ID: "100", ChannelID: "c123", Content: content, Author: discordUser{ID: "bot", Bot: true}}})
		case r.Method == http.MethodPost:
			atomic.AddInt32(&posts, 1)
			http.Error(w, "unexpected post", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Ask(context.Background(), question)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 0 {
		t.Fatalf("posts = %d, want 0", posts)
	}
	if response.Text != "use staging" || response.QuestionMessageID != "100" {
		t.Fatalf("response = %#v, want resumed question 100", response)
	}
}

func TestDiscordAdapterRecoversReplyBeyondFirstHistoryPage(t *testing.T) {
	t.Parallel()

	key := "application=release-lab;decision=old-answer"
	question := Question{Prompt: "Which target?", IdempotencyKey: key, AllowedUserIDs: []string{"u1"}, Timeout: time.Second}
	newer := make([]discordMessage, 100)
	for i := range newer {
		newer[i] = discordMessage{ID: fmt.Sprintf("%d", 250-i), ChannelID: "c123", Content: "newer chatter", Author: discordUser{ID: "u2"}}
	}
	older := []discordMessage{
		{ID: "101", ChannelID: "c123", Content: "use observation", Author: discordUser{ID: "u1"}, MessageReference: &discordMessageReference{MessageID: "100", ChannelID: "c123"}},
		{ID: "100", ChannelID: "c123", Content: FormatQuestionMessage(question), Author: discordUser{ID: "bot", Bot: true}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/users/@me" {
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/channels/c123/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("before") == "151" {
			_ = json.NewEncoder(w).Encode(older)
			return
		}
		_ = json.NewEncoder(w).Encode(newer)
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Ask(context.Background(), question)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "use observation" || response.QuestionMessageID != "100" {
		t.Fatalf("response = %#v, want recovered old reply", response)
	}
}

func TestDiscordAdapterRejectsSameKeyWithDifferentQuestion(t *testing.T) {
	t.Parallel()

	key := "application=release-lab;decision=one"
	original := FormatQuestionMessage(Question{Prompt: "Original?", IdempotencyKey: key})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case "/channels/c123/messages":
			_ = json.NewEncoder(w).Encode([]discordMessage{{ID: "100", Content: original, Author: discordUser{ID: "bot", Bot: true}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Ask(context.Background(), Question{Prompt: "Changed?", IdempotencyKey: key, AllowedUserIDs: []string{"u1"}, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "different question content") {
		t.Fatalf("error = %v, want reused-key content rejection", err)
	}
}

func TestDiscordAdapterRejectsDeduplicatedPostWithDifferentQuestion(t *testing.T) {
	t.Parallel()

	key := "application=release-lab;decision=race"
	existing := FormatQuestionMessage(Question{Prompt: "Other concurrent wording?", IdempotencyKey: key})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(discordMessage{ID: "100", ChannelID: "c123", Content: existing, Author: discordUser{ID: "bot", Bot: true}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Ask(context.Background(), Question{Prompt: "Our wording?", IdempotencyKey: key, AllowedUserIDs: []string{"u1"}, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "different question content") {
		t.Fatalf("error = %v, want deduplicated-post content rejection", err)
	}
}

func TestDiscordAdapterIgnoresForeignBotMarker(t *testing.T) {
	t.Parallel()

	key := "application=release-lab;decision=one"
	var posts int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"our-bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_ = json.NewEncoder(w).Encode([]discordMessage{{ID: "90", Content: FormatQuestionMessage(Question{Prompt: "Changed?", IdempotencyKey: key}), Author: discordUser{ID: "foreign-bot", Bot: true}}})
		case r.Method == http.MethodPost:
			atomic.AddInt32(&posts, 1)
			_ = json.NewEncoder(w).Encode(discordMessage{ID: "100", ChannelID: "c123", Author: discordUser{ID: "our-bot", Bot: true}})
			cancel()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = adapter.Ask(ctx, Question{Prompt: "Original?", IdempotencyKey: key, AllowedUserIDs: []string{"u1"}})
	if posts != 1 {
		t.Fatalf("posts = %d, want 1 after ignoring foreign bot", posts)
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
