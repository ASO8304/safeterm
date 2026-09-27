# Test Matrix

این جدول نگاشت بین Requirement، کامپوننت، نوع تست و نتیجه را نگه می‌دارد.
هدف: از روی این یک فایل بتوان فهمید هر نیازمندی با کدام تست‌ها پوشش داده شده.

| ID | Requirement | Component | Type | Status |
|---|---|---|---|---|
| TEST-CFG-001 | REQ-000 | internal/config | Unit | PASS |
| TEST-CONTRACT-001 | REQ-001 (AC-1) | internal/contract | Unit | PASS |
| TEST-CONTRACT-002 | REQ-001 (AC-2) | internal/contract | Unit | PASS |
| TEST-CONTRACT-003 | REQ-001 (AC-3) | internal/contract | Unit | PASS |
| TEST-CONTRACT-004 | REQ-001 (AC-4) | internal/contract | Unit | PASS |
| TEST-CONTRACT-005 | REQ-001 (AC-5) | internal/contract | Unit | PASS |
| TEST-CONTRACT-006 | REQ-001 (AC-6) | internal/contract | Unit | PASS |
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

شواهد اجرای محلی M5: [خروجی کامل make verify](../../evidence/M5/make-verify.log).
شواهد اجرای محلی M6: [خروجی کامل make verify](../../evidence/M6/make-verify.log).

> این جدول باید با هر Milestone به‌روزرسانی شود. در آینده می‌توان یک اسکریپت
> نوشت که این جدول را مستقیم از خروجی `go test -json` تولید کند تا دستی
> از هم‌گام نیفتد.
