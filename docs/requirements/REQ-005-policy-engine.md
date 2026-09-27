# REQ-005: Policy Engine باید مجوز اجرای عملیات را بر اساس نقش‌های ادعاشده بررسی کند

**Source:** M5 — Policy Engine، قرارداد `agent.PolicyEnforcer` و پیکربندی نقش‌ها و policy در M3
**Priority:** Critical
**Status:** Done

## Description
پکیج `internal/policy` جایگزین `AllowAllPolicy` در مسیر یکپارچه‌ی M5 می‌شود.
این پکیج نگاشت نقش به عملیات مجاز را از Config بارگذاری‌شده و معتبر می‌سازد
و در نبود مجوز صریح، درخواست را رد می‌کند (fail-closed).

`Metadata.ClaimedRoles` حتی پس از M5 هم ادعای تأییدنشده است. Policy فقط
مجوز نقش ادعاشده را بررسی می‌کند؛ احراز هویت و استخراج نقش معتبر وظیفه‌ی
لایه‌ای مستقل در آینده است (ADR-006).

## Acceptance Criteria
- [x] **AC-1:** درخواست بدون نقش (nil یا لیست خالی) رد شود.
- [x] **AC-2:** نقش دارای مجوز پذیرفته و نقش فاقد مجوز رد شود؛ مطابق config واقعی،
  viewer فقط system_info و netops هر دو عملیات system_info و trace را اجراپذیر می‌داند.
- [x] **AC-3:** در چند نقش، وجود حداقل یک نقش مجاز کافی باشد؛ ترتیب نقش‌ها و
  نقش‌های تکراری نتیجه را تغییر ندهند. نقش ناشناخته مجوز نمی‌دهد و مجوز نقش دیگر را لغو نمی‌کند.
- [x] **AC-4:** نقش ناشناخته یا خالی به‌تنهایی هیچ دسترسی ندهد.
- [x] **AC-5:** نقش تعریف‌شده بدون policy، policy با عملیات خالی و Config بدون policy، دسترسی ندهند.
- [x] **AC-6:** چند policy برای یک نقش، مجوزها را با هم تجمیع کنند؛ مجوزها بین نقش‌ها نشت نکنند.
  نگاشت ساخته‌شده مستقل از تغییرات بعدی Config باشد.
- [x] **AC-7:** با Config معتبر، عملیات ناشناخته یا نام خام دستور سیستم مجوز نگیرد.
- [x] **AC-8:** فراخوانی مستقیم با درخواست nil، Config nil یا Enforcer با مقدار صفر،
  دسترسی ایجاد نکند؛ درخواست nil به‌جای panic با خطا رد شود.
- [x] **AC-9:** در Core با Validator و Policy واقعی و config واقعی، مسیر مجاز به
  مراحل بعد برسد؛ رد Policy با نتیجه rejected و مرحله policy، پیش از Translation/Execution
  متوقف شود؛ ورودی نامعتبر نیز پیش از Policy متوقف شود.

## Related Tests
- TEST-POLICY-001 تا TEST-POLICY-014 (`internal/policy/enforcer_test.go`)
- TEST-INTEG-004 تا TEST-INTEG-006 (`tests/integration/agent_policy_test.go`)

## Related ADRs
- ADR-003 (ترتیب ثابت pipeline و توقف بعد از شکست)
- ADR-006 (مجوزهای جمع‌شونده و مرز احراز هویت)
