# M15 Verification Evidence

M15 is verified at two levels:

1. `make verify` runs formatting, vet, build, unit, integration, security
   regression, and end-to-end packages across the repository.
2. `make e2e` runs the deployment bundle/preflight tests and a real SSH client
   through the restricted terminal, authenticated Unix IPC, Gateway, Runner,
   executable-integrity boundary, direct process execution, and durable Audit
   store on the hosted Linux runner.

The release candidate also validates all deployment scripts with `sh -n`,
checks that bundle verification precedes host mutation, and checks the required
service startup order. The root-only systemd demonstration remains an operator
procedure for a fresh disposable Linux host because a hosted CI runner does not
run the repository as the host init system.

## Reproduce

```bash
make verify
make e2e
sudo deploy/demo/run-demo.sh
```

The first two commands are required CI checks for the M15 pull request. Their
immutable GitHub Actions run links are recorded here before the milestone tag
is published.
