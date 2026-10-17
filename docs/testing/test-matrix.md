# Test Matrix

This table maintains the mapping between requirements, components, test types, and results.
Its purpose is to show which tests cover every requirement from a single file.

| ID | Requirement | Component | Type | Status |
|---|---|---|---|---|
| TEST-CFG-001 | REQ-000 | internal/config | Unit | PASS |
| TEST-CONTRACT-001 | REQ-001 (AC-1) | internal/contract | Unit | PASS |
| TEST-CONTRACT-002 | REQ-001 (AC-2) | internal/contract | Unit | PASS |
| TEST-CONTRACT-003 | REQ-001 (AC-3) | internal/contract | Unit | PASS |
| TEST-CONTRACT-004 | REQ-001 (AC-4) | internal/contract | Unit | PASS |
| TEST-CONTRACT-005 | REQ-001 (AC-5) | internal/contract | Unit | PASS |
| TEST-CONTRACT-006 | REQ-001 (AC-6) | internal/contract | Unit | PASS |
| TEST-CONTRACT-007 | REQ-001 (AC-7) | internal/contract | Unit | PASS |
| TEST-AGENT-001 | REQ-002 (AC-5) | internal/agent | Unit | PASS |
| TEST-AGENT-002 | REQ-002 (AC-1) | internal/agent | Unit | PASS |
| TEST-AGENT-003 | REQ-002 (AC-2) | internal/agent | Unit | PASS |
| TEST-AGENT-004 | REQ-002 (AC-3) | internal/agent | Unit | PASS |
| TEST-AGENT-005 | REQ-002 (AC-4) | internal/agent | Unit | PASS |
| TEST-AGENT-006 | REQ-002 (AC-6) | internal/agent | Unit | PASS |
| TEST-AGENT-007 | REQ-002 (pipeline wiring) | internal/agent | Integration | PASS |
| TEST-CONFIG-001..010 | REQ-003 (AC-1..AC-6) | internal/config | Unit | PASS |
| TEST-CONFIG-011..015 | REQ-003 (AC-7..AC-11) | internal/config | Unit | PASS |
| TEST-CONFIG-016 | REQ-003 (shipped config regression) | internal/config | Integration | PASS |
| TEST-VAL-001..013 | REQ-004 (AC-1..AC-8) | internal/validation | Unit | PASS |
| TEST-INTEG-001..003 | REQ-002 + REQ-004 (end-to-end) | tests/integration | Integration | PASS |
| TEST-POLICY-001 | REQ-005 (AC-1) | internal/policy | Unit | PASS |
| TEST-POLICY-002..003 | REQ-005 (AC-2) | internal/policy | Unit | PASS |
| TEST-POLICY-004 | REQ-005 (AC-4) | internal/policy | Unit | PASS |
| TEST-POLICY-005..006 | REQ-005 (AC-3) | internal/policy | Unit | PASS |
| TEST-POLICY-007 | REQ-005 (AC-2) | internal/policy | Unit | PASS |
| TEST-POLICY-008 | REQ-005 (AC-3, AC-4) | internal/policy | Unit | PASS |
| TEST-POLICY-009 | REQ-005 (AC-5) | internal/policy | Unit | PASS |
| TEST-POLICY-010 | REQ-005 (AC-6) | internal/policy | Unit | PASS |
| TEST-POLICY-011 | REQ-005 (AC-7) | internal/policy | Unit | PASS |
| TEST-POLICY-012 | REQ-005 (AC-8) | internal/policy | Unit | PASS |
| TEST-POLICY-013 | REQ-005 (AC-2) | internal/policy | Unit | PASS |
| TEST-POLICY-014 | REQ-005 (AC-6) | internal/policy | Unit | PASS |
| TEST-INTEG-004 | REQ-005 (AC-2, AC-3, AC-9) + REQ-002 (AC-5, AC-6) | tests/integration | Integration | PASS |
| TEST-INTEG-005 | REQ-005 (AC-1..AC-4, AC-9) + REQ-002 (AC-2, AC-6) | tests/integration | Integration | PASS |
| TEST-INTEG-006 | REQ-005 (AC-9) + REQ-002 (AC-1) + REQ-004 (AC-1, AC-3, AC-4, AC-6) | tests/integration | Integration | PASS |
| TEST-TRANS-001 | REQ-006 (AC-1) | internal/translation | Unit | PASS |
| TEST-TRANS-002 | REQ-006 (AC-2) | internal/translation | Unit | PASS |
| TEST-TRANS-003 | REQ-006 (AC-3) | internal/translation | Unit | PASS |
| TEST-TRANS-004 | REQ-006 (AC-4) | internal/translation | Unit | PASS |
| TEST-TRANS-005 | REQ-006 (AC-5) | internal/translation | Unit | PASS |
| TEST-TRANS-006 | REQ-006 (AC-6) | internal/translation | Unit | PASS |
| TEST-TRANS-007 | REQ-006 (AC-7) | internal/translation | Unit | PASS |
| TEST-TRANS-008 | REQ-006 (AC-8) | internal/translation | Unit | PASS |
| TEST-TRANS-009 | REQ-006 (AC-9) | internal/translation | Unit | PASS |
| TEST-INTEG-007 | REQ-006 (AC-1, AC-3, AC-10) + REQ-002 (AC-5, AC-6) | tests/integration | Integration | PASS |
| TEST-INTEG-008 | REQ-006 (AC-11) + REQ-004 (AC-1, AC-2, AC-4, AC-6, AC-7) + REQ-005 (AC-1, AC-2, AC-4) | tests/integration | Integration | PASS |
| TEST-INTEG-009 | REQ-006 (AC-6, AC-11) + REQ-002 (AC-3) | tests/integration | Integration | PASS |
| TEST-EXEC-001 | REQ-007 (AC-1) | internal/execution | Unit | PASS |
| TEST-EXEC-002 | REQ-007 (AC-2) | internal/execution | Unit | PASS |
| TEST-EXEC-003 | REQ-007 (AC-3) | internal/execution | Unit | PASS |
| TEST-EXEC-004 | REQ-007 (AC-4) | internal/execution | Unit | PASS |
| TEST-EXEC-005 | REQ-007 (AC-5) | internal/execution | Unit | PASS |
| TEST-EXEC-006 | REQ-007 (AC-6) | internal/execution | Unit | PASS |
| TEST-EXEC-007 | REQ-007 (AC-7) | internal/execution | Unit | PASS |
| TEST-INTEG-010 | REQ-007 (AC-8) | tests/integration | Integration | PASS |
| TEST-INTEG-011 | REQ-007 (AC-2, AC-10) + REQ-002 (AC-4) | tests/integration | Integration | PASS |
| TEST-INTEG-012 | REQ-007 (AC-4, AC-10) + REQ-002 (AC-4) | tests/integration | Integration | PASS |
| TEST-INTEG-013 | REQ-007 (AC-9) + REQ-004 (AC-1, AC-4, AC-6) + REQ-005 (AC-1, AC-4) | tests/integration | Integration | PASS |
| TEST-ISOLATION-001 | REQ-008 (AC-1) | internal/config | Unit | PASS |
| TEST-ISOLATION-002 | REQ-008 (AC-2) | internal/config, internal/isolation | Unit | PASS |
| TEST-ISOLATION-003 | REQ-008 (AC-3) | deploy/systemd, tests/integration | Integration | PASS |
| TEST-ISOLATION-004 | REQ-008 (AC-4) | internal/isolation, tests/integration | Unit + Integration | PASS |
| TEST-ISOLATION-005 | REQ-008 (AC-5) | internal/config, deploy/systemd | Unit + Integration | PASS |
| TEST-ISOLATION-006 | REQ-008 (AC-6) | internal/isolation, tests/integration | Unit + Integration | PASS |
| TEST-ISOLATION-007 | REQ-008 (AC-7) | internal/execution | Unit | PASS |
| TEST-ISOLATION-008 | REQ-008 (AC-8) | internal/config, internal/translation | Unit | PASS |
| TEST-ISOLATION-009 | REQ-008 (AC-9) | internal/isolation | Unit | PASS |
| TEST-ISOLATION-010 | REQ-008 (AC-10) | internal/isolation, tests/integration | Unit + Integration | PASS |
| TEST-ISOLATION-011 | REQ-008 (AC-11) | internal/execution, tests/integration | Unit + Integration | PASS |
| TEST-AUDIT-001 | REQ-009 (AC-1, AC-9) | internal/audit, tests/integration | Unit + Integration | PASS |
| TEST-AUDIT-002 | REQ-009 (AC-2, AC-3) | internal/audit | Unit | PASS |
| TEST-AUDIT-003 | REQ-009 (AC-6) | internal/audit | Unit | PASS |
| TEST-AUDIT-004 | REQ-009 (AC-7) | internal/audit | Unit | PASS |
| TEST-AUDIT-005 | REQ-009 (AC-5) | internal/audit | Unit | PASS |
| TEST-AUDIT-006 | REQ-009 (AC-8) | internal/isolation | Unit | PASS |
| TEST-AUDIT-007 | REQ-009 (AC-4, AC-9) | deploy/systemd, tests/integration | Integration | PASS |
| TEST-AUDIT-008 | REQ-009 (AC-5, AC-7) | internal/audit | Unit | PASS |
| TEST-INTEGRITY-001 | REQ-010 (AC-4, AC-5) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-002 | REQ-010 (AC-3) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-003 | REQ-010 (AC-4, AC-6) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-004 | REQ-010 (AC-5) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-005 | REQ-010 (AC-5) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-006 | REQ-010 (AC-4, AC-6) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-007 | REQ-010 (AC-4, AC-6, AC-12) | tests/integration | Integration | PASS |
| TEST-INTEGRITY-008 | REQ-010 (AC-6, AC-12) | tests/integration | Integration | PASS |
| TEST-INTEGRITY-009 | REQ-010 (AC-2, AC-9) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-010 | REQ-010 (AC-9) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-011 | REQ-010 (AC-2) | internal/integrity | Unit | PASS |
| TEST-INTEGRITY-012 | REQ-010 (AC-11) | internal/integrity | Integration | PASS |
| TEST-INTEGRITY-013 | REQ-010 (AC-7) | internal/integrity, internal/execution | Unit | PASS |
| TEST-INTEGRITY-014 | REQ-010 (AC-2, AC-13) | internal/isolation | Unit | PASS |
| TEST-INTEGRITY-015 | REQ-010 (AC-10) | cmd/catalog-sign | Unit | PASS |
| TEST-INTEGRITY-016 | REQ-010 (AC-1) | internal/config | Unit | PASS |
| TEST-INTEGRITY-017 | REQ-010 (AC-8) | internal/isolation | Unit | PASS |
| TEST-STREAM-001..006 | REQ-011 (AC-1, AC-3, AC-5, AC-7, AC-8) | internal/contract, internal/execution | Unit | PASS |
| TEST-STREAM-007..013 | REQ-011 (AC-1, AC-6, AC-7, AC-8, AC-11) | internal/isolation | Unit | PASS |
| TEST-STREAM-014..015 | REQ-011 (AC-4, AC-9, AC-12) | internal/isolation | Integration | PASS |
| TEST-STREAM-016..017 | REQ-011 (AC-5, AC-10, AC-13) | internal/execution, tests/integration | Integration | PASS |
| TEST-STREAM-018..024 | REQ-011 (AC-3, AC-4, AC-7, AC-8, AC-9, AC-11, AC-12) | internal/broker, internal/isolation | Unit + Integration | PASS |
| TEST-BROKER-001..002 | REQ-012 (AC-1, AC-2) | internal/broker | Unit | PASS |
| TEST-BROKER-003..005 | REQ-012 (AC-2, AC-3, AC-4, AC-7) | internal/broker | Unit | PASS |
| TEST-BROKER-006..008 | REQ-012 (AC-1, AC-5, AC-9, AC-11) | internal/broker | Live SSH integration | PASS |
| TEST-BROKER-009 | REQ-012 (AC-8) | internal/broker | Unit | PASS |
| TEST-BROKER-010 | REQ-012 (AC-2, AC-6, AC-10, AC-11) | tests/integration | Integration | PASS |
| TEST-BROKER-011 | REQ-012 (AC-6, AC-10) | deploy/systemd, tests/integration | Integration | PASS |
| TEST-BROKER-012 | REQ-012 (AC-8) | internal/broker | Unit | PASS |
| TEST-BROKER-013 | REQ-012 (AC-12) | internal/broker | Unit | PASS |
| TEST-CONFIG-MANAGER-001 | REQ-013 (AC-5, AC-7) | internal/configmanager adapters | Unit | PASS |
| TEST-CONFIG-MANAGER-002 | REQ-013 (AC-2, AC-4, AC-5, AC-8) | internal/configmanager registry and validation | Unit | PASS |
| TEST-CONFIG-MANAGER-003 | REQ-013 (AC-3, AC-9, AC-10, AC-11, AC-14) | internal/configmanager lifecycle | Unit | PASS |
| TEST-CONFIG-MANAGER-004 | REQ-013 (AC-4, AC-13) | internal/configmanager authorization | Unit | PASS |
| TEST-CONFIG-MANAGER-005 | REQ-013 (AC-6, AC-15) | internal/configmanager path validation | Security | PASS |
| TEST-CONFIG-MANAGER-006 | REQ-013 (AC-9, AC-13) | internal/configmanager proposal approval and expiry | Security | PASS |
| TEST-CONFIG-MANAGER-007 | REQ-013 (AC-10, AC-14) | internal/configmanager audit fail-closed | Security | PASS |
| TEST-CONFIG-MANAGER-008 | REQ-013 (AC-12) | internal/configmanager activation health rollback | Unit | PASS |
| TEST-CONFIG-MANAGER-009 | REQ-013 (AC-1, AC-3, AC-13) | internal/configmanager authenticated Unix IPC | Integration | PASS |
| TEST-CONFIG-MANAGER-010 | REQ-013 (AC-3, AC-13) | internal/broker structured config routing | Unit | PASS |
| TEST-CONFIG-MANAGER-011 | REQ-013 (AC-1, AC-2, AC-10) | deploy/systemd and example registry | Integration | PASS |
| TEST-CONFIG-MANAGER-012 | REQ-013 (AC-15) | repository-wide make verify plus Config Manager/Broker/Audit race detection | Regression | PASS |
| TEST-SECURITY-001 | REQ-014 (AC-2) | injection corpus through Validation, Policy, Translation, and Execution boundary | Security | PASS |
| TEST-SECURITY-002 | REQ-014 (AC-3) | production source invariant for direct argv execution without a shell | Security | PASS |
| TEST-SECURITY-003 | REQ-014 (AC-8) | systemd privilege and resource-bound manifests | Security | PASS |
| TEST-SECURITY-004 | REQ-014 (AC-3, AC-7) | signed catalog executable and placeholder constraints | Security | PASS |
| TEST-SECURITY-005..007 | REQ-014 (AC-2, AC-5, AC-10, AC-11) | request, registry, and streaming-frame fuzz targets | Fuzz | PASS |
| TEST-SECURITY-008..010 | REQ-014 (AC-6) | managed config, audit, and Broker hardlink rejection | Security | PASS |
| TEST-SECURITY-011 | REQ-014 (AC-4, AC-5) | Unix peer policy authorization and streaming denial | Integration | PASS |
| TEST-SECURITY-012 | REQ-014 (AC-8, AC-10) | bounded and expiring Broker proposal route cache | Security | PASS |
| TEST-SECURITY-013 | REQ-014 (AC-4) | per-profile Config Manager activation authorization | Security | PASS |
| TEST-SECURITY-014 | REQ-014 (AC-5) | strict Config Manager IPC shapes, unknown fields, and size bounds | Security | PASS |
| TEST-SECURITY-015..017 | REQ-014 (AC-8) | bounded connection admission for Gateway/Runner, Audit, and Config Manager Unix IPC | Security | PASS |
| TEST-SECURITY-018 | REQ-014 (AC-1) | threat-model structure and control-to-verification table conformance | Security | PASS |
| TEST-AUDIT-001..008, TEST-SSH-BROKER-001..007 | REQ-014 (AC-9) | secret-safe audit events, producer binding, hash-chain verification, and terminal-byte filtering | Security | PASS |
| M14-RACE-GATE | REQ-014 (AC-12) | repository-wide race detector including concurrent audit, session, proposal, and IPC state | CI / Security | PASS |
| M14-HOSTED-SECURITY | REQ-014 (AC-13) | adversarial/race, govulncheck, dependency integrity, and zero-finding CodeQL jobs | CI / Security | PASS |
| M14-EVIDENCE-CONFORMANCE | REQ-014 (AC-14) | non-empty verification, race/security, fuzz, and hosted vulnerability evidence plus complete AC mapping | Security | PASS |
| TEST-DEPLOY-001 | REQ-015 (AC-1, AC-2) | catalog preparation and offline signing separation | Unit | PASS |
| TEST-DEPLOY-002 | REQ-015 (AC-3, AC-4) | signed bundle preflight and tamper rejection | Integration | PASS |
| TEST-DEPLOY-003 | REQ-015 (AC-3, AC-5, AC-7) | executable deployment assets and complete bundle contents | Integration | PASS |
| TEST-DEPLOY-004 | REQ-015 (AC-4..AC-7) | pre-mutation verification, dry run, installed preflight, and startup order | Integration | PASS |
| TEST-E2E-001..004 | REQ-015 (AC-8..AC-10) | real SSH, Unix IPC, Gateway, Runner, integrity, execution, and correlated Audit path | End-to-End | PASS |
| TEST-E2E-005..006 | REQ-015 (AC-10..AC-12) | operator demo public workflow, host-key verification, preflight, and documented scope | End-to-End | PASS |
| M15-HOSTED-E2E | REQ-015 (AC-8..AC-12) | isolated hosted deployment and SSH end-to-end CI job | CI / End-to-End | PASS |
| TEST-TERMINAL-001 | REQ-016 (AC-1..AC-4) | prompt, cursor editing, deletion, and bounded history | Unit | PASS |
| TEST-TERMINAL-002 | REQ-016 (AC-4) | role-filtered operation and parameter completion | Security | PASS |
| TEST-CONSOLE-001 | REQ-016 (AC-6..AC-7) | OS-user identity mapping and explicit console broker type | Security | PASS |
| TEST-CONSOLE-002 | REQ-016 (AC-8..AC-9) | bundle, installer, peer groups, and preflight assets | Integration | PASS |
| TEST-PACKAGE-001 | REQ-017 (AC-1..AC-10) | offline builder/installer syntax, completeness, pre-mutation checks, and upgrade preservation invariants | Integration | PASS |
| TEST-PACKAGE-001 | REQ-018 (AC-1..AC-10) | transactional snapshot integrity, owned-path cleanup, account and systemd restoration, and packaged rollback command | Integration | PASS |
| TEST-OPERATOR-001 | REQ-019 (AC-1..AC-4) | mapped log alias validation, fixed-path translation, positional shell syntax, and streaming profile assets | Unit / Integration | PASS |
| TEST-OPERATOR-002 | REQ-019 (AC-5..AC-9) | typed systemd actions, fixed sudo prefix, generated sudoers policy, role split, templates, and guided help | Security / Integration | PASS |
| TEST-ANSIBLE-001 | REQ-020 (AC-1..AC-11) | one-command wrapper, dependency modes, transferred package checksums, dry-run ordering, idempotence, configuration, verification, and rollback assets | Integration | PASS |

Local M5 evidence: [complete make verify output](../../evidence/M5/make-verify.log).
Local M6 evidence: [complete make verify output](../../evidence/M6/make-verify.log).
Local M7 evidence: [complete make verify output](../../evidence/M7/make-verify.log).
Local M8 evidence: [complete make verify output](../../evidence/M8/make-verify.log).
Local M9 evidence: [complete make verify output](../../evidence/M9/make-verify.log).
Local M10 evidence: [complete make verify output](../../evidence/M10/make-verify.log).
Local M11 evidence: [complete make verify output](../../evidence/M11/make-verify.log).
Local M12 evidence: [complete make verify output](../../evidence/M12/make-verify.log).
Local M13 evidence: [complete make verify output](../../evidence/M13/make-verify.log).
Local M14 evidence: [verification, adversarial, race, fuzz, and vulnerability outputs](../../evidence/M14/README.md).
M15 evidence: [verification and hosted deployment/SSH end-to-end outputs](../../evidence/M15/README.md).

> Update this table with every milestone. In the future, a script could generate
> it directly from `go test -json` output to prevent manual drift.


## Preserved site configuration (REQ-023 / ADR-027)

| Test | Behavior | Location |
| --- | --- | --- |
| TestPortableCatalogPreservesCustomizationAndExcludesHostSecrets | Customized model export without host identities or signing material | internal/deployment/upgrade_linux_test.go |
| TestUpgradePreservesSignatureAndApprovedHashes | Signature-preserving upgrade and no implicit executable approval | internal/deployment/upgrade_linux_test.go |
| TestInstallConfigurationPreservesSiteFilesAndRemovesStaleCatalogFragments | Preserve site secrets and modes; remove stale fragments; reject linked directories | internal/deployment/upgrade_linux_test.go |
| TestUpgradeStagesCustomResourceWithoutChangingApplicationMetadata | Stage an external custom INI resource without modifying the live file | internal/deployment/upgrade_linux_test.go |
| TestUpgradeRejectsIncompatibleProfileBeforePreparation | Reject unsupported sandbox migrations | internal/deployment/upgrade_linux_test.go |
| TestPortableCatalogAppliesWithTargetTrustAndExplicitHashRefresh | Target signing, deliberate rehash, dry run, log aliases and rollback | cmd/termctl/catalog_manage_linux_test.go |
| TestUpdatePreservesExistingCatalogUnlessReplacementIsExplicit | Execute package installer without local-key dependency by default | cmd/termctl/main_test.go |


## Guided release installation (REQ-024 / ADR-028)

| Test | Behavior | Location |
| --- | --- | --- |
| TestInstallerGuidedEnrollmentAndUnattendedBoundary | Username defaults/validation, explicit credentials, no nonterminal prompting | tests/e2e/installer_enrollment_test.go |
| TestInstallerPasswordFailureCanBeCancelled | Cancel failed enrollment without persisting bootstrap hash | tests/e2e/installer_enrollment_test.go |
| TestInstallerDependencyPolicyNeverMutatesDuringDryRunOrImplicitAutomation | No APT on dry run or implicit noninteractive installs; explicit/guided APT and repository failure | tests/e2e/installer_enrollment_test.go |
| TestTerminalPasswordIsHiddenConfirmedAndValidated | Real PTY hidden input, confirmation, complexity and private hash files | cmd/password-hash/terminal_linux_test.go |
