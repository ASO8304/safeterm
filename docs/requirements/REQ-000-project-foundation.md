# REQ-000: پروژه باید از روز اول قابل build، تست و verify باشد

**Source:** روش‌شناسی توسعه (M0 — Project Foundation)
**Priority:** Critical
**Status:** In Progress

## Description
پیش از پیاده‌سازی هر قابلیت امنیتی (Validation، Policy، Translation و ...)،
باید یک اسکلت پروژه وجود داشته باشد که با یک دستور واحد (`make verify`)
قابل build، لینت و تست خودکار باشد؛ و این فرآیند در CI هم به‌صورت خودکار
روی هر push اجرا شود.

## Acceptance Criteria
- [x] ساختار پوشه‌بندی پروژه ایجاد شده
- [x] `go.mod` و ماژول پایه ایجاد شده
- [x] یک پکیج نمونه (`internal/config`) با حداقل یک تست واحد وجود دارد
- [x] `make verify` به‌صورت محلی PASS می‌شود
- [ ] GitHub Actions روی اولین push سبز می‌شود (بعد از push توسط کاربر تایید می‌شود)
- [ ] اولین commit و اولین git tag (`v0.1.0-m0`) ثبت شده

## Related Tests
- TEST-CFG-001 (`internal/config/config_test.go`)

## Related ADRs
- ADR-001 (زبان، محیط توسعه، CI/CD)
