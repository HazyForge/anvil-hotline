.PHONY: test build verify install

test:
	go test ./...

build:
	go build -trimpath -ldflags="-s -w" -o bin/anvil-hotline ./cmd/anvil-hotline

verify: test build

install: build
	install -m 0755 bin/anvil-hotline "$(HOME)/.local/bin/anvil-hotline"
	# Compatibility alias for runner images and older AgentRun profiles.
	ln -sfn "$(HOME)/.local/bin/anvil-hotline" "$(HOME)/.local/bin/anvil-agent-feedback"
