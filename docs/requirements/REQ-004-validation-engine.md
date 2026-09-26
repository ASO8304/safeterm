# REQ-004: Validation Engine باید آرگومان‌های درخواست را در برابر تعریف پارامترها بررسی کند

**Source:** سند پروژه، بخش‌های ۴ (مثال trace/tcpdump) و ۶.۳ (Validation) —
«برای هر پارامتر ورودی، قواعد اعتبارسنجی مشخصی تعریف می‌کند... بازه ۱ تا ۶۵۵۳۵...
مستقیماً وارد raw command نشود»
**Priority:** Critical
**Status:** Done

## Description
Validation Engine جایگزین `PassThroughValidator` پلیس‌هولدر M2 می‌شود. برای هر
`ExecutionRequest`، عملیات متناظر را در config (بارگذاری‌شده در M3) پیدا می‌کند
و آرگومان‌های ارسالی را در برابر `ParameterDefinition` هر پارامتر بررسی می‌کند.

دو تصمیم امنیتی محوری (جزئیات در ADR-005):
1. **Whitelist آرگومان‌ها:** هر آرگومانی که در تعریف عملیات نیامده، رد می‌شود.
2. **Defense-in-Depth:** صرف‌نظر از `Pattern` تعریف‌شده، هر مقدار رشته‌ای در
   برابر یک denylist از کاراکترهای خطرناک شل (`;`, `|`, `&`, `` ` ``, `$`, ...)
   بررسی می‌شود؛ و تطبیق `Pattern` باید روی کل رشته باشد، نه فقط بخشی از آن.

## Acceptance Criteria
- [x] **AC-1:** عملیات ناموجود در config رد شود
- [x] **AC-2:** آرگومان الزامی (`required`) که ارسال نشده رد شود
- [x] **AC-3:** پارامتر `port` (نمونه دقیق سند پروژه): ۸۰ ✅، ۰ ❌، ۶۵۵۳۵ ✅،
  ۶۵۵۳۶ ❌، `"abc"` ❌
- [x] **AC-4:** تلاش تزریق دستور در `port` (`"; rm -rf /"`، نمونه دقیق سند
  پروژه) رد شود
- [x] **AC-5:** مقدار `host` که با الگوی IPv4 مطابقت ندارد رد شود
- [x] **AC-6:** آرگومانی که در تعریف عملیات نیامده (whitelist) رد شود
- [x] **AC-7:** کاراکترهای خطرناک شل، حتی وقتی از نظر تئوری `Pattern` ممکن بود
  اجازه عبورشان را بدهد، همیشه رد شوند (defense-in-depth)
- [x] **AC-8:** تطبیق `Pattern` باید روی کل مقدار باشد، نه فقط پیشوند آن —
  حتی اگر نویسنده‌ی config فراموش کند pattern را با `^...$` انکور کند

## Related Tests
- TEST-VAL-001 تا TEST-VAL-013 (`internal/validation/validator_test.go`)
- TEST-INTEG-001 تا TEST-INTEG-003 (`tests/integration/agent_validation_test.go`) —
  اتصال واقعی M2+M3+M4 روی مثال دقیق `trace 192.168.10.20:999` از سند پروژه

## Related ADRs
- ADR-005 (تصمیمات امنیتی Validation Engine: whitelist و defense-in-depth)
