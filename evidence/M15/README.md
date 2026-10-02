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

The successful release-candidate runs are retained by GitHub Actions:

- [Build, Vet, Test and Deployment/SSH End-to-End](https://github.com/ASO8304/safeterm/actions/runs/36994902183)
- [Adversarial/race, vulnerability, dependency, and CodeQL gates](https://github.com/ASO8304/safeterm/actions/runs/36994902152)

These runs execute commit `9cccb46`, including the assertion correction made
after the first hosted test exposed a mismatch in the deployment-asset test.
