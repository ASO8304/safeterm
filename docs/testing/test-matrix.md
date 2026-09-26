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

> این جدول باید با هر Milestone به‌روزرسانی شود. در آینده می‌توان یک اسکریپت
> نوشت که این جدول را مستقیم از خروجی `go test -json` تولید کند تا دستی
> از هم‌گام نیفتد.
