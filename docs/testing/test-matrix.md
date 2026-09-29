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

Local M5 evidence: [complete make verify output](../../evidence/M5/make-verify.log).
Local M6 evidence: [complete make verify output](../../evidence/M6/make-verify.log).
Local M7 evidence: [complete make verify output](../../evidence/M7/make-verify.log).
Local M8 evidence: [complete make verify output](../../evidence/M8/make-verify.log).
Local M9 evidence: [complete make verify output](../../evidence/M9/make-verify.log).

> Update this table with every milestone. In the future, a script could generate
> it directly from `go test -json` output to prevent manual drift.
