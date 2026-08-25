# anvil-hotline

Standalone Go library and CLI for **Anvil Hotline** calls from Anvil agents.

When an agent has gathered evidence and still cannot choose a safe next action,
it posts one narrow question to Discord, waits for an authorized human reply,
and continues only with that reply. The transport is swappable; Discord is the
first implementation.

## Tool name

| Surface | Name |
| --- | --- |
| CLI | `anvil-hotline` |
| Go module | `github.com/hazyforge/anvil-hotline` |
| Library package | `hotline` |

## Install

```bash
go install github.com/hazyforge/anvil-hotline/cmd/anvil-hotline@latest
```

Or build from a checkout:

```bash
go build -o anvil-hotline ./cmd/anvil-hotline
```

## Ask and wait

```bash
export ANVIL_HOTLINE_DISCORD_BOT_TOKEN=...
export ANVIL_HOTLINE_DISCORD_CHANNEL_ID=...
export ANVIL_HOTLINE_ALLOWED_USER_IDS=357735082519429122

# Typed reply (default)
reply="$(anvil-hotline ask \
  --question "May I proceed with the proposed default? Reply yes or no." \
  --context "run=${ANVIL_HOTLINE_RUN:-local-test}" \
  --idempotency-key "application=example;decision=proposed-default" \
  --timeout 30m)"
printf '%s\n' "$reply"

# Or pre-apply emoji choices so the human can click instead of typing
reply="$(anvil-hotline ask \
  --question "May I proceed with the proposed default?" \
  --yes-no-reactions \
  --timeout 30m)"
# returns "yes" or "no" when an authorized user clicks ✅ or ❌
```

Stdout is only the human reply text (or mapped reaction value; JSON with
`--output json`). Errors go to stderr. Secrets must never be printed.

## Decision threads

Use a thread when the human may need evidence or several follow-up exchanges
before they can make a decision. The configured Discord channel remains the
parent authority boundary: thread reads, replies, waits, and final questions
fail closed if `--thread-id` is not directly under that parent.

```bash
thread_json="$(anvil-hotline thread open \
  --title "Execution contract proposal" \
  --message "Review proposed spec digest sha256:abc123." \
  --context "This discussion is not approval." \
  --idempotency-key "spec:execution:abc123")"
thread_id="$(printf '%s' "$thread_json" | jq -r .id)"

# Read the trusted transcript. Messages from other bots or users outside the
# configured allowlist are omitted. nextAfter is the cursor for later reads.
anvil-hotline thread messages --thread-id "$thread_id"

# Continue the discussion without duplicating a reply after agent restart.
anvil-hotline thread reply \
  --thread-id "$thread_id" \
  --message "Here is the requested crash-recovery sequence." \
  --idempotency-key "spec:execution:abc123:recovery-answer-v1"

# Block until the next authorized human message that this bot has not
# already marked addressed. After handling it, ack so later waits skip it.
msg_json="$(anvil-hotline thread wait \
  --thread-id "$thread_id" \
  --timeout 4h)"
msg_id="$(printf '%s' "$msg_json" | jq -r .id)"
anvil-hotline thread ack \
  --thread-id "$thread_id" \
  --message-id "$msg_id"

# List only human replies this bot has not addressed yet.
anvil-hotline thread messages --thread-id "$thread_id" --unaddressed

# A discussion never grants approval. Ask one final, exact question in the
# same thread and bind it to the immutable proposal digest.
anvil-hotline ask \
  --thread-id "$thread_id" \
  --question "Approve spec proposal sha256:abc123 exactly as presented?" \
  --reaction "✅=approve" \
  --reaction "🛠️=request-changes" \
  --reaction "❌=reject" \
  --idempotency-key "spec:execution:abc123:final-decision" \
  --output json
```

`thread open`, `thread messages`, `thread reply`, `thread wait`, and
`thread ack` default to JSON output. `thread open` and keyed replies are
restart-safe. A thread starter uses Discord's enforced nonce plus a
bot-authored semantic marker; because a message-started Discord thread has
the same ID as its starter message, a retry can recover a thread created
immediately before a caller crash.

`thread wait` returns the next allowlisted human message that does **not**
already have this bot's addressed reaction (default `✅`). After the agent
handles that message, `thread ack` applies that reaction. Later waits and
`--unaddressed` reads skip it, so the same reply is not processed twice
without sharing a cursor. Override the emoji with `--addressed-emoji` or
`ANVIL_HOTLINE_ADDRESSED_EMOJI`. Wait does not ack automatically: a crash
after wait and before ack will return the same message again.

Thread posts do not need to be Discord "Reply" references. Empty-content
messages are still skipped because Discord did not give the bot a body.

The bot needs `VIEW_CHANNEL`, `READ_MESSAGE_HISTORY`, `CREATE_PUBLIC_THREADS`,
`SEND_MESSAGES_IN_THREADS`, and `ADD_REACTIONS` in the configured parent
channel. Discord may automatically unarchive and join a visible thread when
the bot sends a message.

### Emoji reaction choices

When you pass reaction options, the bot posts the question, **pre-applies**
those emojis on the message, and accepts either:

1. An authorized user **reacting** with one of the offered emojis, or
2. An authorized user **replying** with typed text (same as before)

| Flag / env | Purpose |
| --- | --- |
| `--yes-no-reactions` / `ANVIL_HOTLINE_YES_NO_REACTIONS=true` | Pre-apply `✅=yes` and `❌=no` |
| `--reaction emoji=value` / `ANVIL_HOTLINE_REACTIONS` | Custom choices; may be repeated or comma-separated |

Examples:

```bash
anvil-hotline ask --question "Ship the release?" --yes-no-reactions

anvil-hotline ask --question "Pick a path" \
  --reaction "✅=proceed" \
  --reaction "❌=abort" \
  --reaction "🔄=retry"

# equivalent via env
ANVIL_HOTLINE_REACTIONS='✅=proceed,❌=abort,🔄=retry' \
  anvil-hotline ask --question "Pick a path"
```

Stdout is the mapped value (`yes`, `proceed`, …). With `--output json`, the
payload includes `"source":"reaction"` and `"reactionEmoji"` when a reaction
was chosen.

## Environment

| Variable | Purpose |
| --- | --- |
| `ANVIL_HOTLINE_DISCORD_BOT_TOKEN` | Bot token |
| `ANVIL_HOTLINE_DISCORD_CHANNEL_ID` | Channel to post into |
| `ANVIL_HOTLINE_ALLOWED_USER_IDS` | Comma-separated Discord user IDs allowed to answer |
| `ANVIL_HOTLINE_ALLOW_ANY_USER` | If `true`, any non-bot member may answer (private channels only) |
| `ANVIL_HOTLINE_TIMEOUT` | Default wait (e.g. `30m`, `1h`) |
| `ANVIL_HOTLINE_POLL_INTERVAL` | Poll cadence (default `5s`) |
| `ANVIL_HOTLINE_ADDRESSED_EMOJI` | Emoji this bot applies to a handled human thread reply (default `✅`) |
| `ANVIL_HOTLINE_ACCEPT_ANY_AFTER` | Accept first non-bot message after the question without requiring a Discord reply reference |
| `ANVIL_HOTLINE_YES_NO_REACTIONS` | If `true`, pre-apply `✅=yes` and `❌=no` on the question |
| `ANVIL_HOTLINE_REACTIONS` | Comma-separated `emoji=value` choices (e.g. `✅=yes,❌=no`) |
| `ANVIL_HOTLINE_TRANSPORT` | Transport name (default `discord`) |
| `ANVIL_HOTLINE_OUTPUT` | Output format: `text` or `json` |
| `ANVIL_HOTLINE_RUN` | Optional run name shown in the Discord message |
| `ANVIL_HOTLINE_IDEMPOTENCY_KEY` | Stable semantic question key used to deduplicate concurrent Discord posts by the same bot |
| `ANVIL_HOTLINE_DISCORD_API_BASE_URL` | Optional Discord API base URL override |

## Fail-closed allowlist

By default the tool refuses to wait unless at least one allowed Discord user
ID is configured. Enable `ANVIL_HOTLINE_ALLOW_ANY_USER=true` only when channel
membership itself is the authorization boundary.

## Library use

```go
import "github.com/hazyforge/anvil-hotline/hotline"

adapter, err := hotline.NewDiscordAdapter(hotline.DiscordConfig{
    BotToken:  token,
    ChannelID: channelID,
})
// ...
svc := hotline.Service{Transport: adapter}
resp, err := svc.Ask(ctx, hotline.Question{
    Prompt:         "Proceed?",
    AllowedUserIDs: []string{"357735082519429122"},
    Timeout:        30 * time.Minute,
    Reactions: []hotline.ReactionOption{
        {Emoji: "✅", Value: "yes"},
        {Emoji: "❌", Value: "no"},
    },
})
// resp.Text is "yes" or "no" for a reaction, or free text for a typed reply.
```

## Agent guidance

Use the hotline only after evidence gathering, with one narrow question, a
proposed default, and a clear expected answer form. Prefer `--yes-no-reactions`
(or explicit `--reaction` choices) when the answer is a short discrete choice so
the human can click instead of typing. Supply a stable `--idempotency-key` for
automated calls. The Discord adapter converts it to an enforced nonce so
concurrent retries from the same bot and channel reuse the first post. It also
embeds a hash marker and searches channel history before posting, so a restarted
caller resumes waiting on the existing question and can recover an already
answered reply. Recovery accepts only a message authored by the currently
authenticated bot with the same canonical question and choices; volatile run
name/context may differ on restart, while reusing a key for a different question
fails closed. Persist the semantic key in the caller's durable run status
before invoking the CLI. JSON output includes `questionMessageId` for the
recovered or newly posted prompt. For an ambiguous decision, open one stable
thread and continue from its `nextAfter` cursor rather than posting disconnected
questions. Treat the transcript as information only. Require a final explicit
`ask --thread-id` decision bound to the immutable proposal or action digest;
changing that proposal invalidates the approval. A reply does not expand
Kubernetes, GitHub, or trading authority.

## Security and release

```bash
make verify
make security   # govulncheck + gosec (also Primaris release gate)
```

- **GitHub Actions:** `security.yml` on PR/push/schedule; `release.yml` on `v*`
  tags requires security before publishing binaries.
- **Anvil Primaris:** `.hazyforge/tests.yaml` release gate suites
  `unit` + `security`. Drive with `anvilctl test run` / ApplicationRelease.

Details: [docs/security-and-release.md](docs/security-and-release.md).
