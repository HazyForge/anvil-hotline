.PHONY: test build verify security security-govulncheck security-gosec install

test:
	go test ./...

build:
	go build -trimpath -ldflags="-s -w" -o bin/anvil-hotline ./cmd/anvil-hotline

verify: test build

# Local/Primaris security gate (mirrored in GitHub Actions and .hazyforge/tests.yaml).
# Prefer the go.mod toolchain (go1.26.5+) so stdlib CVE scans match CI.
security-govulncheck:
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...
	$(MAKE) build
	govulncheck -mode=binary ./bin/anvil-hotline

security-gosec:
	@command -v gosec >/dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec -quiet -exclude=G104 ./...

security: security-govulncheck security-gosec

install: build
	install -m 0755 bin/anvil-hotline "$(HOME)/.local/bin/anvil-hotline"
	# Compatibility alias for runner images and older AgentRun profiles.
	ln -sfn "$(HOME)/.local/bin/anvil-hotline" "$(HOME)/.local/bin/anvil-agent-feedback"
