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

reply="$(anvil-hotline ask \
  --question "May I proceed with the proposed default? Reply yes or no." \
  --context "run=${ANVIL_HOTLINE_RUN:-local-test}" \
  --timeout 30m)"
printf '%s\n' "$reply"
```

Stdout is only the human reply text (or JSON with `--output json`). Errors go
to stderr. Secrets must never be printed.

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
| `ANVIL_HOTLINE_TRANSPORT` | Transport name (default `discord`) |
| `ANVIL_HOTLINE_OUTPUT` | Output format: `text` or `json` |
| `ANVIL_HOTLINE_RUN` | Optional run name shown in the Discord message |
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
