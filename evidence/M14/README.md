# M14 Verification Evidence

M14 verifies the security invariants in REQ-014 against the threat model and
retains the complete local command output used for the milestone decision.

| Evidence | Scope |
|---|---|
| `make-verify.log` | formatting, vet, build, unit, integration, and security seed tests |
| `security.log` | dedicated adversarial suite and repository-wide race detector |
| `fuzz.log` | bounded live campaigns for request, registry, and stream-frame fuzz targets |
| `govulncheck-ci.log` | hosted Go vulnerability reachability analysis for all packages |

The GitHub Security workflow independently repeats the adversarial and race
suite, verifies module checksums, runs govulncheck, and runs CodeQL while
retaining its SARIF artifact. GitHub Dependency Review also runs when enabled
for the repository. The pull request checks are the hosted evidence because
those services do not have a fully equivalent offline mode.

The initial local verification, security, and fuzz commands ran on Linux under
WSL with Go 1.22 before the scanner-mandated dependency upgrade. The project
now requires Go 1.25; CI verifies the final dependency graph and code with Go
1.25. The official govulncheck action uses Go 1.26.8 because the current
scanner requires Go 1.26 or newer. Every recorded command must exit zero; any
scanner finding or race blocks the M14 release.
