
---
trigger: always_on
description: Testing standards, table-driven tests, race detection, and CI verification rules.
---
# Testing & Verification Rules

- **No Broken Tests**: Every code modification in `internal/` MUST be accompanied by updated unit tests in the corresponding `*_test.go` file.
- **Verification Gate**: Before concluding any task, run:
  1. `go vet ./...`
  2. `go test -race -v ./...`
- **Table-Driven Tests**: Write deterministic, table-driven tests using `github.com/stretchr/testify/assert` and `require`.
- **Mocking & Fakes**: Use in-memory fakes implementing Port interfaces (`KubernetesPort`, `LoadBalancerAdapter`, `Logger`, `Metrics`) or `httptest.NewServer` for HTTP adapters (`NginxClient`). Avoid sleeping (`time.Sleep`) in tests where channels, `require.Eventually`, or deterministic triggers can be used.
