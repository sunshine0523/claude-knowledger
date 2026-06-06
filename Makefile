GO ?= go

.PHONY: test test-sqlite run

test:
	$(GO) test ./...

test-sqlite:
	CGO_ENABLED=1 $(GO) test -tags fts5 ./...

run:
	$(GO) run ./cmd/knowledger --config knowledger.yaml
