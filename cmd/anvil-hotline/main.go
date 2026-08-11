package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hazyforge/anvil-hotline/hotline"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			*s = append(*s, item)
		}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "anvil-hotline: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "ask" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("anvil-hotline ask", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var allowedUserIDs stringList
	var reactionFlags stringList
	var attachFlags stringList
	question := flags.String("question", "", "question text to post")
	questionFile := flags.String("question-file", "", "file containing question text, or '-' for stdin")
	contextText := flags.String("context", "", "optional context text")
	contextFile := flags.String("context-file", "", "file containing context text, or '-' for stdin")
	runName := flags.String("run", envFirst("ANVIL_HOTLINE_RUN"), "optional run name for message context")
	transport := flags.String("transport", envFirstDefault("discord", "ANVIL_HOTLINE_TRANSPORT"), "hotline transport")
	timeoutRaw := flags.String("timeout", envFirstDefault("30m", "ANVIL_HOTLINE_TIMEOUT"), "maximum wait time; use 0 for no timeout")
	pollRaw := flags.String("poll-interval", envFirstDefault("5s", "ANVIL_HOTLINE_POLL_INTERVAL"), "poll interval")
	output := flags.String("output", envFirstDefault("text", "ANVIL_HOTLINE_OUTPUT"), "output format: text or json")
	acceptAnyAfter := flags.Bool("accept-any-after", envBoolFirst(false, "ANVIL_HOTLINE_ACCEPT_ANY_AFTER"), "accept first non-bot message after the question instead of requiring a direct reply")
	allowAnyUser := flags.Bool("allow-any-user", envBoolFirst(false, "ANVIL_HOTLINE_ALLOW_ANY_USER"), "allow replies from any non-bot channel member; disabled by default")
	yesNoReactions := flags.Bool("yes-no-reactions", envBoolFirst(false, "ANVIL_HOTLINE_YES_NO_REACTIONS"), "pre-apply ✅=yes and ❌=no reaction choices")
	designReview := flags.Bool("design-review", envBoolFirst(false, "ANVIL_HOTLINE_DESIGN_REVIEW"), "design mockup review: attach images and wait for a free-text Discord reply (no reaction wait)")
	feedbackStyle := flags.String("feedback-style", envFirstDefault("default", "ANVIL_HOTLINE_FEEDBACK_STYLE"), "message style: default or design")
	flags.Var(&allowedUserIDs, "allowed-user-id", "allowed Discord user id; may be repeated or comma-separated")
	flags.Var(&reactionFlags, "reaction", "pre-applied emoji choice as emoji=value (e.g. ✅=yes); may be repeated or comma-separated")
	flags.Var(&attachFlags, "attach", "local file path to attach to the question (repeatable; images render inline in Discord)")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord channel id")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if envAllowed := envFirst("ANVIL_HOTLINE_ALLOWED_USER_IDS"); envAllowed != "" {
		_ = allowedUserIDs.Set(envAllowed)
	}
	if envReactions := envFirst("ANVIL_HOTLINE_REACTIONS"); envReactions != "" {
		_ = reactionFlags.Set(envReactions)
	}
	if envAttach := envFirst("ANVIL_HOTLINE_ATTACH"); envAttach != "" {
		_ = attachFlags.Set(envAttach)
	}

	prompt, err := readTextInput(*question, *questionFile, stdin)
	if err != nil {
		return err
	}
	contextBody, err := readOptionalTextInput(*contextText, *contextFile, stdin)
	if err != nil {
		return err
	}
	timeout, err := parseDurationOrZero(*timeoutRaw)
	if err != nil {
		return fmt.Errorf("parse timeout: %w", err)
	}
	pollInterval, err := time.ParseDuration(*pollRaw)
	if err != nil {
		return fmt.Errorf("parse poll interval: %w", err)
	}

	discordTokenValue := envDefaultIfEmpty(*discordToken, "ANVIL_HOTLINE_DISCORD_BOT_TOKEN")
	discordChannelIDValue := envDefaultIfEmpty(*discordChannelID, "ANVIL_HOTLINE_DISCORD_CHANNEL_ID")
	discordAPIBaseURLValue := envDefaultIfEmpty(*discordAPIBaseURL, "ANVIL_HOTLINE_DISCORD_API_BASE_URL")

	attachments, err := buildAttachments(attachFlags)
	if err != nil {
		return err
	}

	style := strings.TrimSpace(*feedbackStyle)
	// Design review is reply-based: images + free-text. Skip reaction options so
	// the human just types what they like / don't like.
	var reactions []hotline.ReactionOption
	if *designReview {
		if style == "" || style == "default" {
			style = "design"
		}
	} else {
		reactions, err = buildReactionOptions(reactionFlags, *yesNoReactions)
		if err != nil {
			return err
		}
	}

	adapter, err := buildTransport(*transport, discordTokenValue, discordChannelIDValue, discordAPIBaseURLValue, *acceptAnyAfter, *allowAnyUser)
	if err != nil {
		return err
	}
	service := hotline.Service{Transport: adapter}
	response, err := service.Ask(context.Background(), hotline.Question{
		Prompt:         prompt,
		Context:        contextBody,
		RunName:        *runName,
		Timeout:        timeout,
		PollInterval:   pollInterval,
		AllowedUserIDs: allowedUserIDs,
		AcceptAnyAfter: *acceptAnyAfter,
		Reactions:      reactions,
		Attachments:    attachments,
		FeedbackStyle:  style,
	})
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*output)) {
	case "", "text":
		_, err = fmt.Fprintln(stdout, response.Text)
	case "json":
		encoder := json.NewEncoder(stdout)
		err = encoder.Encode(response)
	default:
		err = fmt.Errorf("unsupported output format %q", *output)
	}
	return err
}

func buildTransport(transport, discordToken, discordChannelID, discordAPIBaseURL string, acceptAnyAfter, allowAnyUser bool) (hotline.Transport, error) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", "discord":
		return hotline.NewDiscordAdapter(hotline.DiscordConfig{
			BotToken:       discordToken,
			ChannelID:      discordChannelID,
			APIBaseURL:     discordAPIBaseURL,
			AcceptAnyAfter: acceptAnyAfter,
			AllowAnyUser:   allowAnyUser,
		})
	default:
		return nil, fmt.Errorf("%w: %s", hotline.ErrUnsupportedTransport, transport)
	}
}

func buildReactionOptions(flags stringList, yesNo bool) ([]hotline.ReactionOption, error) {
	var reactions []hotline.ReactionOption
	if yesNo {
		reactions = append(reactions,
			hotline.ReactionOption{Emoji: "✅", Value: "yes"},
			hotline.ReactionOption{Emoji: "❌", Value: "no"},
		)
	}
	for _, raw := range flags {
		option, err := hotline.ParseReactionOption(raw)
		if err != nil {
			return nil, fmt.Errorf("parse reaction %q: %w", raw, err)
		}
		// Explicit --reaction / env entries override the yes/no preset for the same emoji.
		replaced := false
		for i, existing := range reactions {
			if existing.Emoji == option.Emoji {
				reactions[i] = option
				replaced = true
				break
			}
		}
		if !replaced {
			reactions = append(reactions, option)
		}
	}
	return reactions, nil
}

func buildAttachments(paths stringList) ([]hotline.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	attachments := make([]hotline.Attachment, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		attachments = append(attachments, hotline.Attachment{Path: path})
	}
	if len(attachments) == 0 {
		return nil, nil
	}
	return attachments, nil
}

func readTextInput(value, path string, stdin io.Reader) (string, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		return value, nil
	}
	if strings.TrimSpace(path) != "" {
		body, err := readInputPath(path, stdin)
		return strings.TrimSpace(body), err
	}
	if file, ok := stdin.(*os.File); ok {
		info, err := file.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice == 0 {
			body, err := io.ReadAll(stdin)
			return strings.TrimSpace(string(body)), err
		}
	}
	return "", fmt.Errorf("question is required; pass --question, --question-file, or pipe stdin")
}

func readOptionalTextInput(value, path string, stdin io.Reader) (string, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		return value, nil
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	body, err := readInputPath(path, stdin)
	return strings.TrimSpace(body), err
}

func readInputPath(path string, stdin io.Reader) (string, error) {
	path = strings.TrimSpace(path)
	if path == "-" {
		body, err := io.ReadAll(stdin)
		return string(body), err
	}
	// Path is an explicit CLI/operator input (--context-file / path arg), not
	// untrusted path concatenation. Callers choose the file deliberately.
	body, err := os.ReadFile(path) // #nosec G304 -- intentional operator-supplied path
	return string(body), err
}

func parseDurationOrZero(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	return time.ParseDuration(value)
}

func envFirst(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func envFirstDefault(fallback string, names ...string) string {
	if value := envFirst(names...); value != "" {
		return value
	}
	return fallback
}

func envDefaultIfEmpty(value string, names ...string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return envFirst(names...)
}

func envBoolFirst(fallback bool, names ...string) bool {
	for _, name := range names {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}


