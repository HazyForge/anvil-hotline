package hotline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscordAdapterOpensRecoverableThreadFromMessage(t *testing.T) {
	t.Parallel()

	var starterPosts int32
	var threadPosts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","username":"hotline","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/channels/c123/messages":
			atomic.AddInt32(&starterPosts, 1)
			var payload discordCreateMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(payload.Content, "decision thread") || !strings.Contains(payload.Content, "information only") {
				t.Fatalf("starter content = %q, want thread and approval boundary", payload.Content)
			}
			if payload.Nonce == "" || !payload.EnforceNonce {
				t.Fatalf("starter nonce=%q enforce=%v, want enforced nonce", payload.Nonce, payload.EnforceNonce)
			}
			_ = json.NewEncoder(w).Encode(discordMessage{ID: "100", ChannelID: "c123", Content: payload.Content, Author: discordUser{ID: "bot", Bot: true}})
		case r.Method == http.MethodGet && r.URL.Path == "/channels/100":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/channels/c123/messages/100/threads":
			atomic.AddInt32(&threadPosts, 1)
			var payload discordStartThreadRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Name != "Execution contract" || payload.AutoArchiveDuration != 1440 {
				t.Fatalf("thread payload = %+v", payload)
			}
			_ = json.NewEncoder(w).Encode(discordChannel{ID: "100", ParentID: "c123", GuildID: "g1", Name: payload.Name, Type: 11})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := (Service{Transport: adapter}).OpenThread(context.Background(), ThreadRequest{
		Title:           "Execution contract",
		Message:         "Review proposal sha256:abc.",
		IdempotencyKey:  "spec:execution:abc",
		AutoArchiveMins: 1440,
		AllowedUserIDs:  []string{"u1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if thread.ID != "100" || thread.ParentChannelID != "c123" || thread.StarterMessageID != "100" {
		t.Fatalf("thread = %+v", thread)
	}
	if starterPosts != 1 || threadPosts != 1 {
		t.Fatalf("starter posts=%d thread posts=%d, want one each", starterPosts, threadPosts)
	}
}

func TestServiceRejectsOversizedThreadStarterInsteadOfTruncating(t *testing.T) {
	t.Parallel()

	adapter, err := NewDiscordAdapter(DiscordConfig{
		BotToken:  "test",
		ChannelID: "c123",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Service{Transport: adapter}).OpenThread(context.Background(), ThreadRequest{
		Title:          "Large proposal",
		Message:        strings.Repeat("x", 2000),
		IdempotencyKey: "spec:large",
		AllowedUserIDs: []string{"u1"},
	})
	if err == nil || !strings.Contains(err.Error(), "thread starter exceeds") {
		t.Fatalf("error = %v, want explicit size rejection", err)
	}
}

func TestServiceRejectsNegativeThreadWaitTimeout(t *testing.T) {
	t.Parallel()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Service{Transport: adapter}).WaitThread(context.Background(), ThreadWaitRequest{
		ThreadID: "t1",
		Timeout:  -time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("error = %v, want negative-timeout rejection", err)
	}
}

func TestDiscordAdapterResumesExistingThread(t *testing.T) {
	t.Parallel()

	request := ThreadRequest{
		Title:           "Execution contract",
		Message:         "Review proposal sha256:abc.",
		IdempotencyKey:  "spec:execution:abc",
		AutoArchiveMins: 1440,
		AllowedUserIDs:  []string{"u1"},
	}
	content := FormatThreadStarterMessage(request)
	var posts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_ = json.NewEncoder(w).Encode([]discordMessage{{
				ID: "100", ChannelID: "c123", Content: content,
				Author: discordUser{ID: "bot", Bot: true},
				Thread: &discordChannel{ID: "100", ParentID: "c123", Name: "Execution contract", Type: 11},
			}})
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
	thread, err := (Service{Transport: adapter}).OpenThread(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if thread.ID != "100" || posts != 0 {
		t.Fatalf("thread=%+v posts=%d, want recovered thread and no posts", thread, posts)
	}
}

func TestDiscordAdapterRejectsChangedThreadContentForSameKey(t *testing.T) {
	t.Parallel()

	original := ThreadRequest{
		Title:           "Execution contract",
		Message:         "Review proposal sha256:abc.",
		IdempotencyKey:  "spec:execution:stable-key",
		AutoArchiveMins: 1440,
		AllowedUserIDs:  []string{"u1"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/c123/messages":
			_ = json.NewEncoder(w).Encode([]discordMessage{{
				ID: "100", ChannelID: "c123", Content: FormatThreadStarterMessage(original),
				Author: discordUser{ID: "bot", Bot: true},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Message = "Review proposal sha256:def."
	_, err = (Service{Transport: adapter}).OpenThread(context.Background(), changed)
	if err == nil || !strings.Contains(err.Error(), "different question content") {
		t.Fatalf("error = %v, want changed-content rejection", err)
	}
}

func TestDiscordThreadMessagesAreParentScopedAndTrustFiltered(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/t1":
			_ = json.NewEncoder(w).Encode(discordChannel{ID: "t1", ParentID: "c123", Name: "Review", Type: 11})
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/t1/messages":
			_ = json.NewEncoder(w).Encode([]discordMessage{
				{ID: "103", ChannelID: "t1", Content: "inject", Author: discordUser{ID: "stranger"}},
				{ID: "102", ChannelID: "t1", Content: "need more evidence", Author: discordUser{ID: "u1", Username: "Austin"}},
				{ID: "101", ChannelID: "t1", Content: "proposal", Author: discordUser{ID: "bot", Bot: true}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := (Service{Transport: adapter}).ThreadMessages(context.Background(), ThreadMessagesRequest{
		ThreadID: "t1", Limit: 100, AllowedUserIDs: []string{"u1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript.Messages) != 2 {
		t.Fatalf("messages = %+v, want trusted bot and allowed human only", transcript.Messages)
	}
	if transcript.Messages[0].Role != "agent" || transcript.Messages[1].Role != "human" {
		t.Fatalf("message roles = %+v", transcript.Messages)
	}
	if transcript.NextAfter != "103" {
		t.Fatalf("nextAfter = %q, want latest raw cursor 103", transcript.NextAfter)
	}
}

func TestDiscordThreadRejectsForeignParent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/channels/t1" {
			_ = json.NewEncoder(w).Encode(discordChannel{ID: "t1", ParentID: "other", Name: "Foreign", Type: 11})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Service{Transport: adapter}).ThreadMessages(context.Background(), ThreadMessagesRequest{
		ThreadID: "t1", Limit: 100, AllowedUserIDs: []string{"u1"},
	})
	if err == nil || !strings.Contains(err.Error(), "outside the configured parent") {
		t.Fatalf("error = %v, want parent-scope rejection", err)
	}
}

func TestDiscordThreadReplyUsesOptionalIdempotency(t *testing.T) {
	t.Parallel()

	var posted []discordCreateMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/t1":
			_ = json.NewEncoder(w).Encode(discordChannel{ID: "t1", ParentID: "c123", Name: "Review", Type: 11})
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/t1/messages":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/channels/t1/messages":
			var payload discordCreateMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			posted = append(posted, payload)
			_ = json.NewEncoder(w).Encode(discordMessage{ID: "200", ChannelID: "t1", Content: payload.Content, Author: discordUser{ID: "bot", Bot: true}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Transport: adapter}
	if _, err := service.ReplyThread(context.Background(), ThreadReplyRequest{ThreadID: "t1", Message: "More evidence", IdempotencyKey: "reply-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplyThread(context.Background(), ThreadReplyRequest{ThreadID: "t1", Message: "Unkeyed reply"}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %d, want 2", len(posted))
	}
	if posted[0].Nonce == "" || !posted[0].EnforceNonce {
		t.Fatalf("keyed reply nonce=%q enforce=%v", posted[0].Nonce, posted[0].EnforceNonce)
	}
	if posted[1].Nonce != "" || posted[1].EnforceNonce {
		t.Fatalf("unkeyed reply nonce=%q enforce=%v, want none", posted[1].Nonce, posted[1].EnforceNonce)
	}
}

func TestDiscordThreadWaitReturnsNextAllowedHumanMessage(t *testing.T) {
	t.Parallel()

	var polls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/t1":
			_ = json.NewEncoder(w).Encode(discordChannel{ID: "t1", ParentID: "c123", Name: "Review", Type: 11})
		case r.Method == http.MethodGet && r.URL.Path == "/users/@me":
			_, _ = w.Write([]byte(`{"id":"bot","bot":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/t1/messages":
			if atomic.AddInt32(&polls, 1) == 1 {
				_, _ = w.Write([]byte(`[{"id":"101","channel_id":"t1","content":"proposal","author":{"id":"bot","bot":true}}]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"102","channel_id":"t1","content":"show the retry path","author":{"id":"u1","username":"Austin"}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDiscordAdapter(DiscordConfig{BotToken: "test", ChannelID: "c123", APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	message, err := (Service{Transport: adapter}).WaitThread(context.Background(), ThreadWaitRequest{
		ThreadID: "t1", Timeout: time.Second, PollInterval: time.Millisecond, AllowedUserIDs: []string{"u1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Text != "show the retry path" || message.Role != "human" {
		t.Fatalf("message = %+v", message)
	}
}
