# ADR-001: Implementation Language, Development Environment, and CI/CD

## Status
Accepted

## Context
SafeTerm needs a development environment that:

- Can run and test real Linux system commands
- Provides automated build, test, and verification from the first commit
- Runs on a resource-constrained laptop (16 GB RAM, Windows)
- Does not require a dedicated server or heavy infrastructure to get started

## Decision
- **Implementation language:** Go, because it is well suited to process execution
  and concurrency, has a strong standard testing library, and compiles to a
  standalone binary.
- **Development environment:** WSL2 + Ubuntu on Windows, providing a real Linux
  kernel without the overhead of a full virtual machine.
- **CI/CD:** Cloud-hosted GitHub Actions, with no need to host GitLab or a local
  runner. The pipeline runs on GitHub servers rather than the developer laptop.

## Alternatives Considered
1. **A full VM (VirtualBox/VMware) plus self-hosted GitLab** — Rejected for M0
   because its high RAM usage (at least 6 GB for GitLab alone) and setup time
   are not justified by its value at this stage. This option may be revisited
   in a future ADR for deeper DevOps practice.
2. **Direct Windows development without WSL** — Rejected because SafeTerm must
   eventually execute Linux system commands such as tcpdump, making tests in a
   real Linux environment necessary.

## Consequences
- Developers must install WSL2 and Go in the Linux environment rather than
  developing directly on Windows.
- CI depends on internet access, which is acceptable because GitHub Actions is cloud-hosted.
- If self-hosted GitLab later becomes useful for DevOps learning, a new ADR
  (for example, ADR-010) will supersede or extend this decision. The decision
  must not change without documentation.
