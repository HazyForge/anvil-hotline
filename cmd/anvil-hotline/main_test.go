package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpReturnsSuccess(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"ask", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run help returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "Usage of anvil-hotline ask:") {
		t.Fatalf("help output = %q, want usage", stderr.String())
	}
}

func TestRunHelpDoesNotPrintEnvBackedDiscordCredentials(t *testing.T) {
	t.Setenv("ANVIL_HOTLINE_DISCORD_BOT_TOKEN", "secret-bot-token")
	t.Setenv("ANVIL_HOTLINE_DISCORD_CHANNEL_ID", "secret-channel-id")
	t.Setenv("ANVIL_HOTLINE_DISCORD_API_BASE_URL", "https://discord.example.invalid/api")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"ask", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run help returned error: %v", err)
	}
	help := stderr.String()
	for _, secret := range []string{"secret-bot-token", "secret-channel-id", "discord.example.invalid"} {
		if strings.Contains(help, secret) {
			t.Fatalf("help output contains env-backed Discord config %q: %q", secret, help)
		}
	}
}

func TestRunRequiresQuestion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"ask"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("run without a question returned nil error")
	}
	if !strings.Contains(err.Error(), "question is required") {
		t.Fatalf("error = %q, want question requirement", err)
	}
}

func TestLegacyAgentFeedbackEnvironmentAliases(t *testing.T) {
	t.Setenv("ANVIL_AGENT_FEEDBACK_TIMEOUT", "17m")
	t.Setenv("ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS", "u1,u2")
	t.Setenv("ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN", "legacy-token")
	t.Setenv("ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID", "legacy-channel")
	if got := envFirstDefault("30m", "ANVIL_HOTLINE_TIMEOUT", "ANVIL_AGENT_FEEDBACK_TIMEOUT"); got != "17m" {
		t.Fatalf("timeout alias = %q, want 17m", got)
	}
	if got := envFirst("ANVIL_HOTLINE_ALLOWED_USER_IDS", "ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS"); got != "u1,u2" {
		t.Fatalf("allowed alias = %q, want u1,u2", got)
	}
	if got := envDefaultIfEmpty("", "ANVIL_HOTLINE_DISCORD_BOT_TOKEN", "ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN"); got != "legacy-token" {
		t.Fatalf("token alias = %q, want legacy-token", got)
	}
	if got := envDefaultIfEmpty("", "ANVIL_HOTLINE_DISCORD_CHANNEL_ID", "ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID"); got != "legacy-channel" {
		t.Fatalf("channel alias = %q, want legacy-channel", got)
	}
}

func TestBuildReactionOptionsYesNoAndOverrides(t *testing.T) {
	t.Parallel()

	reactions, err := buildReactionOptions(stringList{"🔄=retry"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(reactions) != 3 {
		t.Fatalf("len(reactions) = %d, want 3", len(reactions))
	}
	if reactions[0].Emoji != "✅" || reactions[0].Value != "yes" {
		t.Fatalf("first reaction = %+v, want ✅=yes", reactions[0])
	}
	if reactions[1].Emoji != "❌" || reactions[1].Value != "no" {
		t.Fatalf("second reaction = %+v, want ❌=no", reactions[1])
	}
	if reactions[2].Emoji != "🔄" || reactions[2].Value != "retry" {
		t.Fatalf("third reaction = %+v, want 🔄=retry", reactions[2])
	}

	overridden, err := buildReactionOptions(stringList{"✅=ship"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(overridden) != 2 {
		t.Fatalf("len(overridden) = %d, want 2", len(overridden))
	}
	if overridden[0].Value != "ship" {
		t.Fatalf("overridden yes value = %q, want ship", overridden[0].Value)
	}
}

func TestRunHelpMentionsReactionAndIdempotencyFlags(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"ask", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run help returned error: %v", err)
	}
	help := stderr.String()
	for _, want := range []string{"-reaction", "-yes-no-reactions", "-idempotency-key", "-thread-id"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q: %q", want, help)
		}
	}
}

func TestRunThreadHelpListsConversationCommands(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"thread", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("thread help returned error: %v", err)
	}
	for _, want := range []string{"thread <open|messages|reply|wait>", "open", "messages", "reply", "wait"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("thread help missing %q: %q", want, stderr.String())
		}
	}
}

func TestRunThreadOpenHelpDoesNotRequireCredentials(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"thread", "open", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("thread open help returned error: %v", err)
	}
	for _, want := range []string{"-title", "-message", "-idempotency-key", "-auto-archive-minutes"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("thread open help missing %q: %q", want, stderr.String())
		}
	}
}
