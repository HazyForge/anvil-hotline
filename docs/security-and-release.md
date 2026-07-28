# Security scans and Anvil Primaris release gates

`anvil-hotline` is a **public** repository. Security checks run in GitHub
Actions (public minutes) and as Anvil Primaris **TestContract** suites so
release promotion fails closed without a green scan.

## What runs

| Check | Local | GitHub Actions | Primaris TestContract |
| --- | --- | --- | --- |
| `go test` / build | `make verify` | `ci.yml` | suite `unit` |
| govulncheck (module + binary) | `make security` | `security.yml`, `release.yml` | suite `security` |
| gosec | `make security` | `security.yml`, `release.yml` | suite `security` |
| CodeQL | — | `security.yml`, `release.yml` | — (GHA Security tab) |
| Dependency review | — | PRs only | — |
| OpenSSF Scorecard | — | push/schedule/release | — |

## Local

```bash
make verify
make security
```

## GitHub (public Actions)

- **Every PR / push to master:** `ci.yml` + `security.yml`
- **Weekly schedule:** `security.yml` (cron)
- **Each GitHub Release / `v*` tag:** `release.yml` runs govulncheck, gosec, and
  CodeQL **before** uploading binaries

Tag a release only after security is green:

```bash
git tag v0.1.1
git push origin v0.1.1
# release.yml builds multi-arch binaries and publishes the GitHub Release
```

## Drive with Anvil Primaris

Repo contracts (checked into Git):

- `.hazyforge/tests.yaml` — TestContract `anvil-hotline`
  - `gates.release.suites: [unit, security]`
- `.hazyforge/release.yaml` — `testGateSuites: [unit, security]`
- `.hazyforge/clusters/anvil-primaris/namespace/anvilhub/manifests/application.yaml`
  — Application + Repository so Primaris can bind `anvil-hotline` to the
  TestContract path above

On a Primaris build worker with the repo checkout:

```bash
# Security suite only (govulncheck + gosec via make security)
anvilctl test run --application anvil-hotline --suite security

# Full release gate set required by ApplicationRelease
anvilctl test run --application anvil-hotline --gate release
```

An `ApplicationRelease` for application `anvil-hotline` must list these gate
suites (or inherit from the TestContract `gates.release`). Missing or failed
security evidence must fail the release.

Deployed-image audits of consumers (agent runners that `go install` hotline)
use the existing Primaris skill
`audit-deployed-security-with-trivy` against cluster image digests — that is
cluster posture, not this module’s source release gate.

## Required GitHub settings (once)

For SARIF upload / Scorecard publish on a public repo:

1. **Settings → Code security** — enable Code scanning, Dependency graph,
   Dependabot alerts.
2. Default `GITHUB_TOKEN` permissions allow `security-events: write` as set in
   the workflows.

## Failure policy

- `govulncheck` or `gosec` failures block merge (PR) and block tag release.
- Scorecard publishes findings but is not a hard gate on PR (runs on push).
- Primaris release gates treat suite `security` as required.
