.PHONY: test build verify security security-govulncheck security-gosec install

test:
	go test ./...

build:
	go build -trimpath -ldflags="-s -w" -o bin/anvil-hotline ./cmd/anvil-hotline

verify: test build

# Local/Primaris security gate. go.mod pins a patched Go toolchain so the
# resulting binary does not inherit known standard-library vulnerabilities.
# Resolve tools by exact GOPATH location after installation so a developer does
# not need GOPATH/bin on PATH for the gate to work.
security-govulncheck:
	@set -eu; \
	tool="$$(command -v govulncheck 2>/dev/null || printf '%s/bin/govulncheck' "$$(go env GOPATH)")"; \
	if [ ! -x "$$tool" ]; then GOBIN="$$(go env GOPATH)/bin" go install golang.org/x/vuln/cmd/govulncheck@latest; fi; \
	"$$tool" ./...; \
	$(MAKE) build; \
	"$$tool" -mode=binary ./bin/anvil-hotline

security-gosec:
	@set -eu; \
	tool="$$(command -v gosec 2>/dev/null || printf '%s/bin/gosec' "$$(go env GOPATH)")"; \
	if [ ! -x "$$tool" ]; then GOBIN="$$(go env GOPATH)/bin" go install github.com/securego/gosec/v2/cmd/gosec@latest; fi; \
	"$$tool" -quiet -exclude=G104 ./...

security: security-govulncheck security-gosec

install: build
	install -m 0755 bin/anvil-hotline "$(HOME)/.local/bin/anvil-hotline"
