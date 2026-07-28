# anvil-hotline

Standalone Go library and CLI for **Anvil Hotline** calls from Anvil and Hazy
Trade agents.

When an agent has gathered evidence and still cannot choose a safe next action,
it posts one narrow question to Discord, waits for an authorized human reply,
and continues only with that reply. The transport is swappable; Discord is the
first implementation.

## Tool name

| Surface | Name |
| --- | --- |
| Preferred CLI | `anvil-hotline` |
| Compatibility alias | `anvil-agent-feedback` (optional symlink in runner images) |
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
export ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN=...
export ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID=...
export ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS=357735082519429122

reply="$(anvil-hotline ask \
  --question "May I proceed with the proposed default? Reply yes or no." \
  --context "AgentRun=${ANVIL_AGENT_RUN:-local-test}" \
  --timeout 30m)"
printf '%s\n' "$reply"
```

Stdout is only the human reply text (or JSON with `--output json`). Errors go
to stderr. Secrets must never be printed.

## Environment

| Variable | Purpose |
| --- | --- |
| `ANVIL_HOTLINE_DISCORD_BOT_TOKEN` / `ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN` / `DISCORD_BOT_TOKEN` | Bot token |
| `ANVIL_HOTLINE_DISCORD_CHANNEL_ID` / `ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID` / `DISCORD_CHANNEL_ID` | Channel to post into |
| `ANVIL_HOTLINE_ALLOWED_USER_IDS` / `ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS` | Comma-separated Discord user IDs allowed to answer |
| `ANVIL_HOTLINE_ALLOW_ANY_USER` / `ANVIL_AGENT_FEEDBACK_ALLOW_ANY_USER` | If `true`, any non-bot member may answer (private channels only) |
| `ANVIL_HOTLINE_TIMEOUT` / `ANVIL_AGENT_FEEDBACK_TIMEOUT` | Default wait (e.g. `30m`, `1h`) |
| `ANVIL_HOTLINE_POLL_INTERVAL` / `ANVIL_AGENT_FEEDBACK_POLL_INTERVAL` | Poll cadence (default `5s`) |
| `ANVIL_HOTLINE_ACCEPT_ANY_AFTER` / `ANVIL_AGENT_FEEDBACK_ACCEPT_ANY_AFTER` | Accept first non-bot message after the question without requiring a Discord reply reference |
| `ANVIL_AGENT_RUN` | Optional AgentRun name shown in the Discord message |

Legacy `ANVIL_AGENT_FEEDBACK_*` names remain supported so existing Kubernetes
Secrets keep working.

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
})
```

## Agent guidance

Use the hotline only after evidence gathering, with one narrow question, a
proposed default, and a clear expected answer form. A reply is information; it
does not expand Kubernetes, GitHub, or trading authority.

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

## Origin

Extracted from the Discord ask-and-wait path that lived in `anvil-agents`
(`lib/agentfeedback` / `cmd/anvil-agent-feedback`), itself the runtime home for
the former Anvil Primaris AgentRun feedback helper. This repository is the
source of truth for the binary and library.
