.PHONY: test build verify install

test:
	go test ./...

build:
	go build -trimpath -ldflags="-s -w" -o bin/operator-hotline ./cmd/operator-hotline

verify: test build

install: build
	install -m 0755 bin/operator-hotline "$(HOME)/.local/bin/operator-hotline"
	ln -sfn "$(HOME)/.local/bin/operator-hotline" "$(HOME)/.local/bin/anvil-agent-feedback"
