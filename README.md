# anvil-operator-hotline

Standalone Go library and CLI for **operator hotline** calls from Anvil /
Hazy Trade agents.

When an agent has gathered evidence and still cannot choose a safe next action,
it posts one narrow question to Discord, waits for an authorized human reply,
and continues only with that reply. The transport is swappable; Discord is the
first implementation.

## Tool name

| Surface | Name |
| --- | --- |
| Preferred CLI | `operator-hotline` |
| Compatibility alias | `anvil-agent-feedback` (still installed in runner images) |
| Go module | `github.com/hazyforge/anvil-operator-hotline` |
| Library package | `hotline` |

## Install

```bash
go install github.com/hazyforge/anvil-operator-hotline/cmd/operator-hotline@latest
```

Or build from a checkout:

```bash
go build -o operator-hotline ./cmd/operator-hotline
```

## Ask and wait

```bash
export ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN=...
export ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID=...
export ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS=357735082519429122

reply="$(operator-hotline ask \
  --question "May I proceed with the proposed default? Reply yes or no." \
  --context "AgentRun=${ANVIL_AGENT_RUN:-local-test}" \
  --timeout 30m)"
printf '%s\n' "$reply"
```

Stdout is only the operator reply text (or JSON with `--output json`). Errors
go to stderr. Secrets must never be printed.

## Environment

| Variable | Purpose |
| --- | --- |
| `ANVIL_AGENT_FEEDBACK_DISCORD_BOT_TOKEN` / `DISCORD_BOT_TOKEN` | Bot token |
| `ANVIL_AGENT_FEEDBACK_DISCORD_CHANNEL_ID` / `DISCORD_CHANNEL_ID` | Channel to post into |
| `ANVIL_AGENT_FEEDBACK_ALLOWED_USER_IDS` | Comma-separated Discord user IDs allowed to answer |
| `ANVIL_AGENT_FEEDBACK_ALLOW_ANY_USER` | If `true`, any non-bot member may answer (private channels only) |
| `ANVIL_AGENT_FEEDBACK_TIMEOUT` | Default wait (e.g. `30m`, `1h`) |
| `ANVIL_AGENT_FEEDBACK_POLL_INTERVAL` | Poll cadence (default `5s`) |
| `ANVIL_AGENT_FEEDBACK_ACCEPT_ANY_AFTER` | Accept first non-bot message after the question without requiring a Discord reply reference |
| `ANVIL_AGENT_RUN` | Optional AgentRun name shown in the Discord message |

`OPERATOR_HOTLINE_*` aliases are also accepted for the same settings.

## Fail-closed allowlist

By default the tool refuses to wait unless at least one allowed Discord user
ID is configured. Enable `ANVIL_AGENT_FEEDBACK_ALLOW_ANY_USER=true` only when
channel membership itself is the authorization boundary.

## Library use

```go
import "github.com/hazyforge/anvil-operator-hotline/hotline"

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

## Origin

Extracted from the Discord ask-and-wait path that lived in
`anvil-agents` (`lib/agentfeedback` / `cmd/anvil-agent-feedback`), itself the
runtime home for the former Anvil Primaris AgentRun feedback helper.
