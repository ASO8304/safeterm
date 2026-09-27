# M6 Evidence — Translation Layer

Date: 2026-09-27. Environment: WSL, Go 1.22.2 on Linux/amd64.

## Baseline and Scope
The branch starts from merged M5 commit `03b1fae` on `origin/main`.
[Pre-change baseline verification](baseline-verify.log) completed with
`SafeTerm Verification: PASS`. The separate `/home/abolfazl/safeterm`
checkout and its backup stash were not modified.

Repository inspection showed that, contrary to the continuation prompt,
`system_info` has `args_template: ["-a"]`. That definition was preserved, and
an empty template was tested through an independent fixture. The M5 tag was
also absent from the fetched tag list; README therefore reports merge and tag
status separately.

## Requirement and Decision
- REQ-006 defines eleven acceptance criteria for translation, incomplete-input
  rejection, data independence, and pipeline termination.
- ADR-007 restricts placeholders to complete tokens. Malformed templates fail
  during construction; a missing value is an error even for an optional
  placeholder; arguments are never joined or split.
- `translation.New` returns an error because template construction can fail.
  Stage interfaces and Core behavior remain unchanged. Literals and the command
  come from trusted configuration.
- IdentityTranslator remains in earlier isolated tests. The M6 integration path
  uses the real Validator, Policy, and Translator with a test-only Executor.

## Tests and Result
[Complete make verify output](make-verify.log) is stored without abbreviation.

```text
SafeTerm Verification: PASS
```

- TEST-TRANS-001 through TEST-TRANS-009 cover AC-1 through AC-9 respectively.
- TEST-INTEG-007 verifies the exact command and argv received by the test Executor (AC-10).
- TEST-INTEG-008 verifies that invalid input and unauthorized roles stop before Translation (AC-11).
- TEST-INTEG-009 verifies that an absent optional parameter passes Validation
  but fails Translation without calling Executor (AC-6 and AC-11).
- Nine new unit tests and three new integration tests passed, as did all earlier
  tests. Statement coverage for the translation package is 100%.

## Reproduction
When running the Windows checkout from WSL, Git trust was configured only for
that command and exact path. No global setting or external dependency was added:

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=safe.directory \
GIT_CONFIG_VALUE_0=/mnt/c/Users/abolf/OneDrive/Documents/safeterm \
make verify
```

The branch was created from the M5 baseline with:

```sh
git switch -c codex/m6-translation origin/main
```

## Limitations and Review
Translate assumes Validation and Policy used the same Config and does not
repeat type or authorization checks. Config is a trusted source; interpreting
tool-specific executable options and actually starting a process are outside
this milestone. Authentication, M7, and later milestones have not started.
CI is checked in the PR; merge and the M6 tag occur after owner review.
