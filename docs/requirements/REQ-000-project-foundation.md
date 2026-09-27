# REQ-000: The Project Must Be Buildable, Testable, and Verifiable from Day One

**Source:** Development methodology (M0 — Project Foundation)
**Priority:** Critical
**Status:** In Progress

## Description
Before implementing any security capability such as Validation, Policy, or
Translation, the project must have a skeleton that can be built, linted, and
tested automatically with one command (`make verify`). CI must also run this
process automatically on every push.

## Acceptance Criteria
- [x] Create the project directory structure
- [x] Create `go.mod` and the base module
- [x] Provide a sample package (`internal/config`) with at least one unit test
- [x] `make verify` passes locally
- [ ] GitHub Actions passes on the first push (confirmed after the user pushes)
- [ ] Record the first commit and first Git tag (`v0.1.0-m0`)

## Related Tests
- TEST-CFG-001 (`internal/config/config_test.go`)

## Related ADRs
- ADR-001 (language, development environment, and CI/CD)
