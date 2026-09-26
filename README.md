# SafeTerm

پلتفرم قابل پیکربندی برای اجرای امن دستورات سیستمی
(A Configurable Secure Command Execution Platform)

## وضعیت فعلی

**Milestone: M0 — Project Foundation**

این نسخه هنوز هیچ عملیات واقعی اجرا نمی‌کند. هدف M0 صرفاً این است که:

- ساختار پروژه مشخص باشد
- `make verify` از روز اول سبز باشد
- CI/CD (GitHub Actions) از همان اولین commit فعال باشد

## اجرای محلی

```bash
make verify
```

این دستور به‌ترتیب اجرا می‌کند: بررسی فرمت کد → `go vet` → build → تست‌های واحد.
اگر همه‌چیز سبز بود، خروجی نهایی این است:

```
SafeTerm Verification: PASS
```

## ساختار پروژه

```
safeterm/
├── cmd/            نقاط ورود اجرایی (agent, broker, ...)
├── internal/       منطق داخلی پروژه (config, validation, policy, ...)
├── configs/        فایل‌های پیکربندی (operations, roles, policies)
├── tests/          تست‌های integration/security/e2e
├── docs/           مستندات (requirements, decisions/ADR, testing)
└── evidence/       شواهد اجرای موفق هر Milestone
```

## روش کار (Methodology)

هر قابلیت جدید این چرخه را طی می‌کند:

```
Requirement → Design → Implementation → Unit Test →
Integration Test → Security Test → Evidence → Documentation → Git Tag
```

جزئیات در `docs/decisions/` (Architecture Decision Records) و `docs/requirements/` ثبت می‌شود.

## Milestone Roadmap

| # | عنوان | وضعیت |
|---|---|---|
| M0 | Project Foundation | 🔄 In Progress |
| M1 | ExecutionRequest Contract | ⏳ Planned |
| M2 | Agent Core | ⏳ Planned |
| M3 | Configuration Loading | ⏳ Planned |
| M4 | Validation Engine | ⏳ Planned |
| M5 | Policy Engine | ⏳ Planned |
| M6 | Translation Layer | ⏳ Planned |
| M7 | Command Runner | ⏳ Planned |
| M8 | Audit Logging | ⏳ Planned |
| M9 | Integrity Verification | ⏳ Planned |
| M10 | Streaming | ⏳ Planned |
| M11 | Brokers (CLI/SSH/Web) | ⏳ Planned |
| M12 | Security Testing | ⏳ Planned |
| M13 | End-to-End Demo | ⏳ Planned |
