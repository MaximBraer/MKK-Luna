SHELL := /bin/sh

.PHONY: test-unit test-integration qa-cover qa-e2e qa

test-unit:
	go test ./...

test-integration:
	INTEGRATION=1 go test -tags integration ./tests/integration -v

qa-cover:
	@CRITICAL_PKGS=$$(go list ./internal/service/... ./internal/repository/... | tr '\n' ','); \
	INTEGRATION=1 go test -tags=integration -coverpkg=$$CRITICAL_PKGS -coverprofile=coverage.out ./...; \
	TOTAL=$$(go tool cover -func coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	THRESHOLD=85; \
	awk -v total="$$TOTAL" -v threshold="$$THRESHOLD" 'BEGIN { if (total+0 < threshold+0) { print "Coverage below threshold"; exit 1 } }'

qa-e2e:
	E2E=1 go test -tags e2e ./tests/e2e -v

qa: qa-cover qa-e2e
