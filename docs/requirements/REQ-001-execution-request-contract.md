# REQ-001: قرارداد استاندارد ExecutionRequest بین Broker و Agent

**Source:** سند پروژه، بخش ۷.۳ (قرارداد ارتباطی بین Broker و Agent) — شناسه یکتا،
نام command، آرگومان‌های ساختاریافته و metadata
**Priority:** Critical
**Status:** Done

## Description
هر Broker باید ورودی کاربر را به یک ساختار داده استاندارد (`ExecutionRequest`)
تبدیل کند، پیش از این‌که به Agent Core تحویل بدهد. Agent Core هرگز نباید چیزی
خارج از این قرارداد بپذیرد. Broker مجاز نیست بداند command کاربر به چه دستور
واقعی سیستم map می‌شود — این دانش فقط در Agent (Translation Layer، M6) قرار دارد.

نکته امنیتی مهم (مرتبط با ADR-003): مقادیر داخل `Metadata.ClaimedRoles` صرفاً
ادعای Broker هستند و تا قبل از پیاده‌سازی Policy Engine (M5) نباید به‌عنوان
هویت تایید‌شده در نظر گرفته شوند.

## Acceptance Criteria
- [x] **AC-1:** ساخت یک `ExecutionRequest` بدون Command باید رد شود (خطا برگرداند)
- [x] **AC-2:** هر فراخوانی باید یک `RequestID` یکتا و غیرخالی تولید کند
- [x] **AC-3:** مقدار `nil` برای Arguments باید به یک map خالی نرمالایز شود
- [x] **AC-4:** فیلد `Metadata.ReceivedAt` در صورت خالی بودن، خودکار پر شود
- [x] **AC-5:** `ExecutionRequest` باید بتواند بدون از دست دادن داده، به JSON تبدیل و برگردانده شود (round-trip)
- [x] **AC-6:** `ExecutionResponse.Duration()` باید فاصله زمانی بین شروع و پایان اجرا را درست محاسبه کند

## Related Tests
- TEST-CONTRACT-001 تا TEST-CONTRACT-006 (`internal/contract/request_test.go`, `internal/contract/response_test.go`)

## Related ADRs
- ADR-002 (طراحی قرارداد ExecutionRequest/ExecutionResponse)
