---
name: verification-before-completion
description: "Use when about to claim work is complete, bug is fixed, or tests pass. Requires running verification commands (vet, race, lint, build) and confirming evidence before reporting success."
risk: low
source: community
---

# Verification Before Completion

## Overview

Claiming work is complete without verification is dishonesty, not efficiency.

**Core principle:** Evidence before claims, always.

**Violating the letter of this rule is violating the spirit of this rule.**

## The Iron Law

```
NO COMPLETION CLAIMS WITHOUT FRESH VERIFICATION EVIDENCE
```

If you haven't run the verification command in this message, you cannot claim it passes.

---

## The Verification Gate for mlb-controller

BEFORE claiming any status or reporting success to the user:

1. **IDENTIFY**: What command proves this claim?
2. **RUN**: Execute the FULL verification suite:
   - Code formatting & Static analysis: `make fmt && make vet`
   - Unit tests with race detection: `go test -race -v ./...`
   - Binary compilation: `make build`
   - Linting: `make lint` (if `golangci-lint` is installed)
   - Integration tests (if integration logic changed): `make test-integration`
3. **READ**: Inspect the complete output, check exit code (must be 0), count test failures (must be 0).
4. **VERIFY**: Does the output actually confirm the claim?
   - If NO: State the actual failure with error output and diagnose systematically.
   - If YES: State completion WITH the fresh evidence.
5. **ONLY THEN**: Make the claim.

Skip any step = guessing, not verifying.

---

## Required Verification Matrix

| Claim | Requires Evidence | Insufficient "Evidence" |
|---|---|---|
| "Tests pass" | `go test -race -v ./...` output with 0 failures | Previous run, "code looks right", unit test passed earlier |
| "Bug fixed" | Reproducing test fails before fix, passes cleanly after fix | Code edited without running test |
| "Race condition resolved" | `go test -race -count=1 ./...` exits 0 with no data race warnings | Testing without `-race` flag |
| "Build succeeds" | `make build` produces `bin/mlb-controller` with exit code 0 | "Linter passed" or "only modified comments" |
| "Informer sync bug fixed" | Test with simulated watch events and cache sync passes | Assuming informer handles tombstones |

---

## Red Flags - STOP and Verify

- Thinking or saying "should work now", "probably works", or "looks clean".
- Expressing satisfaction ("Great!", "Done!", "Fixed!") before running tests.
- Relying on partial verification (e.g. running only 1 package instead of `./...`).
- Assuming a refactor didn't break callers without running the full test suite.
- Reporting completion when a background command is still running.

---

## The Bottom Line

**No shortcuts for verification.**

Run the commands. Read the exit codes. Confirm zero failures. THEN report to the user.
