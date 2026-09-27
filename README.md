# SafeTerm

پلتفرم قابل پیکربندی برای اجرای امن دستورات سیستمی
(A Configurable Secure Command Execution Platform)

## وضعیت فعلی

**Milestone: M6 — Translation Layer (پیاده‌سازی و تست محلی تکمیل؛ در انتظار بازبینی و انتشار)**

این نسخه هنوز هیچ عملیات واقعی سیستم‌عامل اجرا نمی‌کند. قابلیت‌های موجود:

- قرارداد درخواست/پاسخ و Core با ترتیب ثابت Validation → Policy → Translation → Execution
- بارگذاری JSON و اعتبارسنجی آرگومان‌ها بر اساس config
- بررسی مجوز نقش‌های ادعاشده با Policy واقعی و تست یکپارچه‌ی M5
- ترجمه‌ی عملیات به دستور ثابت از config و آرگومان‌های جداگانه، بدون ساخت رشته‌ی shell

Policy احراز هویت نمی‌کند؛ `ClaimedRoles` همچنان ادعای تأییدنشده است (ADR-006).
Translation واقعی در M6 پیاده‌سازی شده؛ Execution واقعی مربوط به M7 است. نقطه‌ی ورود
`cmd/agent` هنوز برنامه‌ی bootstrap است؛ اتصال مراحل واقعی فعلاً در تست‌های
یکپارچه اثبات می‌شود. شواهد M6 در [evidence/M6](evidence/M6/README.md) ثبت شده‌اند.

نمونه‌ی خروجی ترجمه برای trace با port=999 و host=192.168.10.20:

```text
command: tcpdump
args: ["-i", "any", "port", "999", "and", "host", "192.168.10.20"]
```

سازنده‌ی `translation.New(cfg)` خطا برمی‌گرداند؛ پیش از اتصال به Core باید
آن را بررسی کرد. قالب فقط literal یا `{name}` کامل را می‌پذیرد و مقدار هر
placeholder باید موجود باشد، حتی اگر پارامتر در Validation اختیاری است (ADR-007).

## اجرای محلی

```bash
make verify
```

این دستور به‌ترتیب اجرا می‌کند: بررسی فرمت کد → `go vet` → build → تست‌های واحد و یکپارچه.
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
| M0 | Project Foundation | ✅ Tagged: v0.1.0-m0 |
| M1 | ExecutionRequest Contract | ✅ Tagged: v0.2.0-m1 |
| M2 | Agent Core | ✅ Tagged: v0.3.0-m2 |
| M3 | Configuration Loading | ✅ Tagged: v0.4.0-m3 |
| M4 | Validation Engine | ✅ Tagged: v0.5.0-m4 |
| M5 | Policy Engine | ✅ Merged: PR #5; tag pending |
| M6 | Translation Layer | 🔄 Verified locally; pending review/release |
| M7 | Command Runner | ⏳ Planned |
| M8 | Audit Logging | ⏳ Planned |
| M9 | Integrity Verification | ⏳ Planned |
| M10 | Streaming | ⏳ Planned |
| M11 | Brokers (CLI/SSH/Web) | ⏳ Planned |
| M12 | Security Testing | ⏳ Planned |
| M13 | End-to-End Demo | ⏳ Planned |
