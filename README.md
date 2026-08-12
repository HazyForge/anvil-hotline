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
recovered or newly posted prompt. A reply is
information; it does not expand Kubernetes, GitHub, or trading authority.

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
