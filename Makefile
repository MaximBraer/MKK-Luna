SHELL := /bin/sh

.PHONY: test-unit test-integration

test-unit:
	go test ./...

test-integration:
	INTEGRATION=1 go test -tags integration ./tests/integration -v
