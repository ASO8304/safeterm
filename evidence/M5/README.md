# M5 Evidence — Policy Engine

Verification date: 2026-09-27. Environment: WSL, `go version go1.22.2 linux/amd64`.

## Initial State and Corrections
- The checkout used for this work was `C:\Users\abolf\OneDrive\Documents\safeterm`.
  The policy file was incorrectly located under `internal/agent`, and the test file was empty.
- A separate `/home/abolfazl/safeterm` checkout had the file in the correct
  directory and contained seven Policy tests. Those tests were brought into
  this checkout and expanded; the separate WSL checkout was not modified.
- Baseline verification initially failed because of the empty test file and
  CRLF line endings. The owner's explicit request to complete M5 authorized
  correcting that state.
- The Policy implementation was preserved, with nil request and nil config
  handling added. Core and its four interfaces did not change in behavior or signature.
- REQ-005 and ADR-006 were added. The test matrix, README, and authentication-
  boundary wording were corrected. `.gitattributes` stabilized Go and Makefile line endings.
- `fmt-check` now propagates gofmt execution failures. The prior version could
  hide a tool failure when it produced no stdout. CI also runs for `codex/**` branches.

## Verification Result
[Complete, unabridged make verify output](make-verify.log):

```text
SafeTerm Verification: PASS
```

All 14 Policy unit tests and three M5 integration tests (covering 14 integration
scenarios) passed. Statement coverage for `internal/policy` is 100%. All tests
from earlier milestones also passed. The exact acceptance-criterion-to-test
mapping is recorded in the [test matrix](../../docs/testing/test-matrix.md).

The [negative fmt-check verification](fmt-check-negative.log) also confirmed
that a fake gofmt returning a failure with empty stdout causes the Make target to fail.

## Reproducing in WSL
Because Windows-drive ownership differs under WSL, Git initially reported
`detected dubious ownership`. For this verification command only, the specific
project path was supplied to Git. No global settings changed and VCS stamping remained enabled:

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=safe.directory \
GIT_CONFIG_VALUE_0=/mnt/c/Users/abolf/OneDrive/Documents/safeterm \
make verify
```

## Scope and Release
This evidence covers local completion of M5. Authentication, the real
Translation layer, and the real Executor were not implemented in this change.
M4 tests retain their placeholder to isolate Validator; M5 tests use the real Policy.

The working branch was created with:

```sh
git switch -c codex/m5-policy-completion
```

PR merge and the M5 tag occur after owner review. A local PASS alone is not
evidence of merge or release; CI status must be checked in the PR.
