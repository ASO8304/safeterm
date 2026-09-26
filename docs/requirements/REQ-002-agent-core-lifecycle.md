# REQ-002: Agent Core باید هر ExecutionRequest را از یک چرخه‌ی ثابت و متوقف‌شونده عبور دهد

**Source:** سند پروژه، بخش‌های ۷.۲ و ۷.۴ (Agent Core به‌عنوان نقطه مرکزی تصمیم‌گیری) و
۸ (مدل امنیتی — Central Policy Enforcement)
**Priority:** Critical
**Status:** Done

## Description
Agent Core باید هر `ExecutionRequest` را دقیقاً به همین ترتیب عبور دهد:
**Validation → Policy → Translation → Execution**. اگر هرکدام از سه مرحله‌ی
اول شکست بخورد، هیچ مرحله‌ی بعدی (و مخصوصاً Execution) نباید فراخوانی شود.

نکته طراحی: چون در زمان ساخت M2 هنوز پیاده‌سازی واقعی Validation (M4)،
Policy (M5) و Translation (M6) وجود ندارد، این چهار مرحله به‌صورت interface
(`Validator`, `PolicyEnforcer`, `Translator`, `Executor`) تعریف شده‌اند.
Agent Core فقط ترتیب و توقف زودهنگام را تضمین می‌کند، نه منطق داخلی هر مرحله.
پیاده‌سازی‌های فعلی (`placeholders.go`) صرفاً برای اثبات صحت pipeline هستند و
**هیچ اعتبار امنیتی ندارند** — باید در M4/M5/M6/M7 با پیاده‌سازی واقعی
جایگزین شوند.

## Acceptance Criteria
- [x] **AC-1:** اگر Validation شکست بخورد، Policy/Translation/Execution هیچ‌کدام فراخوانی نمی‌شوند
- [x] **AC-2:** اگر Policy شکست بخورد، Translation/Execution فراخوانی نمی‌شوند
- [x] **AC-3:** اگر Translation شکست بخورد، Execution فراخوانی نمی‌شود
- [x] **AC-4:** اگر Execution شکست بخورد، نتیجه باید `execution_error` باشد، نه `rejected`
  (تمایز بین «رد شد چون سیاست/اعتبارسنجی اجازه نداد» و «اجرا شد ولی خطا داد»)
- [x] **AC-5:** وقتی همه مراحل موفق باشند، نتیجه باید `success` باشد و `RequestID`
  پاسخ باید دقیقاً با `RequestID` درخواست یکسان باشد
- [x] **AC-6:** فیلدهای `StartedAt`/`EndedAt` باید در تمام مسیرها (موفق و ناموفق) پر شوند،
  چون Audit (M8) به مدت‌زمان هر عملیات نیاز دارد، حتی عملیات ردشده

## Related Tests
- TEST-AGENT-001 تا TEST-AGENT-007 (`internal/agent/core_test.go`)

## Related ADRs
- ADR-003 (طراحی Interface-based pipeline و مدل خطای StageError)
