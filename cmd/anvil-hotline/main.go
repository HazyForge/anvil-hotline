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
	if len(args) > 0 && args[0] == "thread" {
		return runThread(args[1:], stdin, stdout, stderr)
	}
	return runAsk(args, stdin, stdout, stderr)
}

func runAsk(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "ask" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("anvil-hotline ask", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var allowedUserIDs stringList
	var reactionFlags stringList
	question := flags.String("question", "", "question text to post")
	questionFile := flags.String("question-file", "", "file containing question text, or '-' for stdin")
	contextText := flags.String("context", "", "optional context text")
	contextFile := flags.String("context-file", "", "file containing context text, or '-' for stdin")
	runName := flags.String("run", envFirst("ANVIL_HOTLINE_RUN"), "optional run name for message context")
	idempotencyKey := flags.String("idempotency-key", envFirst("ANVIL_HOTLINE_IDEMPOTENCY_KEY"), "stable semantic key used to deduplicate concurrent question posts")
	transport := flags.String("transport", envFirstDefault("discord", "ANVIL_HOTLINE_TRANSPORT", "ANVIL_AGENT_FEEDBACK_TRANSPORT", "ANVIL_SOCIAL_TRANSPORT"), "hotline transport")
	timeoutRaw := flags.String("timeout", envFirstDefault("30m", "ANVIL_HOTLINE_TIMEOUT", "ANVIL_AGENT_FEEDBACK_TIMEOUT"), "maximum wait time; use 0 for no timeout")
	pollRaw := flags.String("poll-interval", envFirstDefault("5s", "ANVIL_HOTLINE_POLL_INTERVAL", "ANVIL_AGENT_FEEDBACK_POLL_INTERVAL"), "poll interval")
	output := flags.String("output", envFirstDefault("text", "ANVIL_HOTLINE_OUTPUT", "ANVIL_AGENT_FEEDBACK_OUTPUT"), "output format: text or json")
	acceptAnyAfter := flags.Bool("accept-any-after", envBoolFirst(false, "ANVIL_HOTLINE_ACCEPT_ANY_AFTER", "ANVIL_AGENT_FEEDBACK_ACCEPT_ANY_AFTER"), "accept first non-bot message after the question instead of requiring a direct reply")
	allowAnyUser := flags.Bool("allow-any-user", envBoolFirst(false, "ANVIL_HOTLINE_ALLOW_ANY_USER", "ANVIL_AGENT_FEEDBACK_ALLOW_ANY_USER"), "allow replies from any non-bot channel member; disabled by default")
	yesNoReactions := flags.Bool("yes-no-reactions", envBoolFirst(false, "ANVIL_HOTLINE_YES_NO_REACTIONS"), "pre-apply ✅=yes and ❌=no reaction choices")
	flags.Var(&allowedUserIDs, "allowed-user-id", "allowed Discord user id; may be repeated or comma-separated")
	flags.Var(&reactionFlags, "reaction", "pre-applied emoji choice as emoji=value (e.g. ✅=yes); may be repeated or comma-separated")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord channel id")
	discordThreadID := flags.String("thread-id", "", "optional Discord thread id under the configured parent channel")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if envAllowed := envFirst("ANVIL_HOTLINE_ALLOWED_USER_IDS", "ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS", "ANVIL_DISCORD_ALLOWED_USER_IDS"); envAllowed != "" {
		_ = allowedUserIDs.Set(envAllowed)
	}
	if envReactions := envFirst("ANVIL_HOTLINE_REACTIONS"); envReactions != "" {
		_ = reactionFlags.Set(envReactions)
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

	discordTokenValue := envDefaultIfEmpty(*discordToken, "ANVIL_HOTLINE_DISCORD_BOT_TOKEN", "ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN", "ANVIL_DISCORD_BOT_TOKEN", "DISCORD_BOT_TOKEN")
	discordChannelIDValue := envDefaultIfEmpty(*discordChannelID, "ANVIL_HOTLINE_DISCORD_CHANNEL_ID", "ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID", "ANVIL_DISCORD_CHANNEL_ID", "DISCORD_CHANNEL_ID")
	discordAPIBaseURLValue := envDefaultIfEmpty(*discordAPIBaseURL, "ANVIL_HOTLINE_DISCORD_API_BASE_URL", "ANVIL_AGENT_FEEDBACK_DISCORD_API_BASE_URL", "DISCORD_API_BASE_URL")

	reactions, err := buildReactionOptions(reactionFlags, *yesNoReactions)
	if err != nil {
		return err
	}

	var adapter hotline.Transport
	if strings.TrimSpace(*discordThreadID) != "" {
		adapter, err = buildThreadTransport(*transport, discordTokenValue, discordChannelIDValue, *discordThreadID, discordAPIBaseURLValue, *acceptAnyAfter, *allowAnyUser)
	} else {
		adapter, err = buildTransport(*transport, discordTokenValue, discordChannelIDValue, discordAPIBaseURLValue, *acceptAnyAfter, *allowAnyUser)
	}
	if err != nil {
		return err
	}
	service := hotline.Service{Transport: adapter}
	response, err := service.Ask(context.Background(), hotline.Question{
		Prompt:         prompt,
		Context:        contextBody,
		RunName:        *runName,
		IdempotencyKey: *idempotencyKey,
		Timeout:        timeout,
		PollInterval:   pollInterval,
		AllowedUserIDs: allowedUserIDs,
		AcceptAnyAfter: *acceptAnyAfter,
		Reactions:      reactions,
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

func runThread(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stderr, "Usage: anvil-hotline thread <open|messages|reply|wait|ack> [flags]")
		return err
	}
	switch args[0] {
	case "open":
		return runThreadOpen(args[1:], stdin, stdout, stderr)
	case "messages":
		return runThreadMessages(args[1:], stdout, stderr)
	case "reply":
		return runThreadReply(args[1:], stdin, stdout, stderr)
	case "wait":
		return runThreadWait(args[1:], stdout, stderr)
	case "ack":
		return runThreadAck(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unsupported thread command %q; use open, messages, reply, wait, or ack", args[0])
	}
}

func runThreadOpen(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("anvil-hotline thread open", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var allowedUserIDs stringList
	title := flags.String("title", "", "thread title (1-100 characters)")
	message := flags.String("message", "", "initial discussion message")
	messageFile := flags.String("message-file", "", "file containing the initial message, or '-' for stdin")
	contextText := flags.String("context", "", "optional context text")
	contextFile := flags.String("context-file", "", "file containing context text, or '-' for stdin")
	runName := flags.String("run", envFirst("ANVIL_HOTLINE_RUN"), "optional run name for message context")
	idempotencyKey := flags.String("idempotency-key", envFirst("ANVIL_HOTLINE_IDEMPOTENCY_KEY"), "stable semantic key used to recover this thread")
	autoArchiveMins := flags.Int("auto-archive-minutes", 1440, "Discord auto-archive duration: 60, 1440, 4320, or 10080")
	output := flags.String("output", "json", "output format: text or json")
	flags.Var(&allowedUserIDs, "allowed-user-id", "allowed Discord user id; may be repeated or comma-separated")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord parent channel id")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")
	allowAnyUser := flags.Bool("allow-any-user", envBoolFirst(false, "ANVIL_HOTLINE_ALLOW_ANY_USER"), "allow any non-bot member in this private channel")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	loadAllowedUserIDs(&allowedUserIDs)
	body, err := readRequiredTextInput(*message, *messageFile, stdin, "thread message")
	if err != nil {
		return err
	}
	contextBody, err := readOptionalTextInput(*contextText, *contextFile, stdin)
	if err != nil {
		return err
	}
	adapter, err := buildDiscordAdapter(
		envDefaultIfEmpty(*discordToken, "ANVIL_HOTLINE_DISCORD_BOT_TOKEN", "ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN"),
		envDefaultIfEmpty(*discordChannelID, "ANVIL_HOTLINE_DISCORD_CHANNEL_ID", "ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID"),
		"",
		envDefaultIfEmpty(*discordAPIBaseURL, "ANVIL_HOTLINE_DISCORD_API_BASE_URL", "ANVIL_AGENT_FEEDBACK_DISCORD_API_BASE_URL"),
		false,
		*allowAnyUser,
	)
	if err != nil {
		return err
	}
	thread, err := (hotline.Service{Transport: adapter}).OpenThread(context.Background(), hotline.ThreadRequest{
		Title:           *title,
		Message:         body,
		Context:         contextBody,
		RunName:         *runName,
		IdempotencyKey:  *idempotencyKey,
		AutoArchiveMins: *autoArchiveMins,
		AllowedUserIDs:  allowedUserIDs,
	})
	if err != nil {
		return err
	}
	return writeStructuredOutput(stdout, *output, thread.ID, thread)
}

func runThreadMessages(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("anvil-hotline thread messages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var allowedUserIDs stringList
	threadID := flags.String("thread-id", "", "Discord thread id")
	afterMessageID := flags.String("after-message-id", "", "return messages after this cursor")
	limit := flags.Int("limit", 100, "maximum messages to read (1-100)")
	unaddressedOnly := flags.Bool("unaddressed", false, "omit human messages this bot already addressed with the addressed emoji")
	addressedEmoji := flags.String("addressed-emoji", envFirstDefault(hotline.DefaultAddressedEmoji, "ANVIL_HOTLINE_ADDRESSED_EMOJI"), "emoji this bot uses to mark a human reply as addressed")
	output := flags.String("output", "json", "output format: text or json")
	flags.Var(&allowedUserIDs, "allowed-user-id", "allowed Discord user id; may be repeated or comma-separated")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord parent channel id")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")
	allowAnyUser := flags.Bool("allow-any-user", envBoolFirst(false, "ANVIL_HOTLINE_ALLOW_ANY_USER"), "allow any non-bot member in this private channel")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	loadAllowedUserIDs(&allowedUserIDs)
	adapter, err := buildDiscordAdapterFromFlags(*discordToken, *discordChannelID, *discordAPIBaseURL, *allowAnyUser)
	if err != nil {
		return err
	}
	transcript, err := (hotline.Service{Transport: adapter}).ThreadMessages(context.Background(), hotline.ThreadMessagesRequest{
		ThreadID:        *threadID,
		AfterMessageID:  *afterMessageID,
		Limit:           *limit,
		AllowedUserIDs:  allowedUserIDs,
		AddressedEmoji:  *addressedEmoji,
		UnaddressedOnly: *unaddressedOnly,
	})
	if err != nil {
		return err
	}
	return writeStructuredOutput(stdout, *output, formatTranscriptText(transcript), transcript)
}

func runThreadReply(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("anvil-hotline thread reply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	threadID := flags.String("thread-id", "", "Discord thread id")
	message := flags.String("message", "", "reply text")
	messageFile := flags.String("message-file", "", "file containing reply text, or '-' for stdin")
	idempotencyKey := flags.String("idempotency-key", envFirst("ANVIL_HOTLINE_IDEMPOTENCY_KEY"), "stable semantic key used to recover this reply")
	output := flags.String("output", "json", "output format: text or json")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord parent channel id")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	body, err := readRequiredTextInput(*message, *messageFile, stdin, "thread reply message")
	if err != nil {
		return err
	}
	adapter, err := buildDiscordAdapterFromFlags(*discordToken, *discordChannelID, *discordAPIBaseURL, false)
	if err != nil {
		return err
	}
	reply, err := (hotline.Service{Transport: adapter}).ReplyThread(context.Background(), hotline.ThreadReplyRequest{
		ThreadID:       *threadID,
		Message:        body,
		IdempotencyKey: *idempotencyKey,
	})
	if err != nil {
		return err
	}
	return writeStructuredOutput(stdout, *output, reply.ID, reply)
}

func runThreadWait(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("anvil-hotline thread wait", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var allowedUserIDs stringList
	threadID := flags.String("thread-id", "", "Discord thread id")
	afterMessageID := flags.String("after-message-id", "", "wait for a human message after this cursor")
	timeoutRaw := flags.String("timeout", envFirstDefault("30m", "ANVIL_HOTLINE_TIMEOUT", "ANVIL_AGENT_FEEDBACK_TIMEOUT"), "maximum wait time; use 0 for no timeout")
	pollRaw := flags.String("poll-interval", envFirstDefault("5s", "ANVIL_HOTLINE_POLL_INTERVAL", "ANVIL_AGENT_FEEDBACK_POLL_INTERVAL"), "poll interval")
	addressedEmoji := flags.String("addressed-emoji", envFirstDefault(hotline.DefaultAddressedEmoji, "ANVIL_HOTLINE_ADDRESSED_EMOJI"), "emoji this bot uses to mark a human reply as addressed")
	output := flags.String("output", "json", "output format: text or json")
	flags.Var(&allowedUserIDs, "allowed-user-id", "allowed Discord user id; may be repeated or comma-separated")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord parent channel id")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")
	allowAnyUser := flags.Bool("allow-any-user", envBoolFirst(false, "ANVIL_HOTLINE_ALLOW_ANY_USER"), "allow any non-bot member in this private channel")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	loadAllowedUserIDs(&allowedUserIDs)
	timeout, err := parseDurationOrZero(*timeoutRaw)
	if err != nil {
		return fmt.Errorf("parse timeout: %w", err)
	}
	pollInterval, err := time.ParseDuration(*pollRaw)
	if err != nil {
		return fmt.Errorf("parse poll interval: %w", err)
	}
	adapter, err := buildDiscordAdapterFromFlags(*discordToken, *discordChannelID, *discordAPIBaseURL, *allowAnyUser)
	if err != nil {
		return err
	}
	message, err := (hotline.Service{Transport: adapter}).WaitThread(context.Background(), hotline.ThreadWaitRequest{
		ThreadID:       *threadID,
		AfterMessageID: *afterMessageID,
		Timeout:        timeout,
		PollInterval:   pollInterval,
		AllowedUserIDs: allowedUserIDs,
		AddressedEmoji: *addressedEmoji,
	})
	if err != nil {
		return err
	}
	return writeStructuredOutput(stdout, *output, message.Text, message)
}

func runThreadAck(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("anvil-hotline thread ack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	threadID := flags.String("thread-id", "", "Discord thread id")
	messageID := flags.String("message-id", "", "human message id to mark addressed")
	addressedEmoji := flags.String("addressed-emoji", envFirstDefault(hotline.DefaultAddressedEmoji, "ANVIL_HOTLINE_ADDRESSED_EMOJI"), "emoji this bot uses to mark a human reply as addressed")
	output := flags.String("output", "json", "output format: text or json")
	discordToken := flags.String("discord-token", "", "Discord bot token")
	discordChannelID := flags.String("discord-channel-id", "", "Discord parent channel id")
	discordAPIBaseURL := flags.String("discord-api-base-url", "", "Discord API base URL")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	adapter, err := buildDiscordAdapterFromFlags(*discordToken, *discordChannelID, *discordAPIBaseURL, false)
	if err != nil {
		return err
	}
	if err := (hotline.Service{Transport: adapter}).AckThread(context.Background(), hotline.ThreadAckRequest{
		ThreadID:  *threadID,
		MessageID: *messageID,
		Emoji:     *addressedEmoji,
	}); err != nil {
		return err
	}
	acked := map[string]string{
		"threadId":  strings.TrimSpace(*threadID),
		"messageId": strings.TrimSpace(*messageID),
		"emoji":     strings.TrimSpace(*addressedEmoji),
	}
	if acked["emoji"] == "" {
		acked["emoji"] = hotline.DefaultAddressedEmoji
	}
	return writeStructuredOutput(stdout, *output, acked["messageId"], acked)
}

func buildDiscordAdapterFromFlags(token, channelID, apiBaseURL string, allowAnyUser bool) (*hotline.DiscordAdapter, error) {
	return buildDiscordAdapter(
		envDefaultIfEmpty(token, "ANVIL_HOTLINE_DISCORD_BOT_TOKEN", "ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN"),
		envDefaultIfEmpty(channelID, "ANVIL_HOTLINE_DISCORD_CHANNEL_ID", "ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID"),
		"",
		envDefaultIfEmpty(apiBaseURL, "ANVIL_HOTLINE_DISCORD_API_BASE_URL", "ANVIL_AGENT_FEEDBACK_DISCORD_API_BASE_URL"),
		false,
		allowAnyUser,
	)
}

func loadAllowedUserIDs(allowedUserIDs *stringList) {
	if envAllowed := envFirst("ANVIL_HOTLINE_ALLOWED_USER_IDS", "ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS", "ANVIL_DISCORD_ALLOWED_USER_IDS"); envAllowed != "" {
		_ = allowedUserIDs.Set(envAllowed)
	}
}

func writeStructuredOutput(stdout io.Writer, output, textValue string, value any) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "text":
		_, err := fmt.Fprintln(stdout, textValue)
		return err
	case "", "json":
		return json.NewEncoder(stdout).Encode(value)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func formatTranscriptText(transcript hotline.ThreadTranscript) string {
	var builder strings.Builder
	for _, message := range transcript.Messages {
		state := "open"
		if message.Addressed {
			state = "addressed"
		}
		fmt.Fprintf(&builder, "%s\t%s\t%s\t%s\n", message.ID, message.Role, state, strings.ReplaceAll(message.Text, "\n", "\\n"))
	}
	return strings.TrimSuffix(builder.String(), "\n")
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

func buildThreadTransport(transport, discordToken, parentChannelID, threadID, discordAPIBaseURL string, acceptAnyAfter, allowAnyUser bool) (hotline.Transport, error) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "", "discord":
		return buildDiscordAdapter(discordToken, threadID, parentChannelID, discordAPIBaseURL, acceptAnyAfter, allowAnyUser)
	default:
		return nil, fmt.Errorf("%w: %s", hotline.ErrUnsupportedTransport, transport)
	}
}

func buildDiscordAdapter(discordToken, discordChannelID, parentChannelID, discordAPIBaseURL string, acceptAnyAfter, allowAnyUser bool) (*hotline.DiscordAdapter, error) {
	return hotline.NewDiscordAdapter(hotline.DiscordConfig{
		BotToken:        discordToken,
		ChannelID:       discordChannelID,
		ParentChannelID: parentChannelID,
		APIBaseURL:      discordAPIBaseURL,
		AcceptAnyAfter:  acceptAnyAfter,
		AllowAnyUser:    allowAnyUser,
	})
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

func readTextInput(value, path string, stdin io.Reader) (string, error) {
	return readRequiredTextInput(value, path, stdin, "question")
}

func readRequiredTextInput(value, path string, stdin io.Reader, label string) (string, error) {
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
	return "", fmt.Errorf("%s is required; pass the matching text/file flag or pipe stdin", label)
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
