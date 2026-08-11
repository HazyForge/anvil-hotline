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

func TestRunHelpMentionsReactionFlags(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"ask", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run help returned error: %v", err)
	}
	help := stderr.String()
	for _, want := range []string{"-reaction", "-yes-no-reactions", "-attach", "-design-review"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q: %q", want, help)
		}
	}
}

func TestBuildAttachments(t *testing.T) {
	t.Parallel()

	got, err := buildAttachments(stringList{" /tmp/a.png ", " /tmp/b.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Path != "/tmp/a.png" || got[1].Path != "/tmp/b.jpg" {
		t.Fatalf("paths = %#v", got)
	}
}
