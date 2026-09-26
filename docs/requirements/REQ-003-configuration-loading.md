# REQ-003: بارگذاری و اعتبارسنجی ساختاری Configuration

**Source:** سند پروژه، بخش ۶.۱ (پیکربندی‌پذیری) — تعریف operation، role و policy
از طریق فایل configuration، بدون تغییر core
**Priority:** Critical
**Status:** Done

## Description
Agent باید بتواند مجموعه‌ی عملیات مجاز (`operations`)، نقش‌ها (`roles`) و
نگاشت نقش-به-عملیات (`policies`) را از فایل‌های config بخواند. این بارگذاری
باید:
1. از چند فایل در هر پوشه پشتیبانی کند (افزودن عملیات جدید = افزودن یک فایل
   جدید، نه ویرایش فایل موجود)
2. در برابر فایل خراب/ناقص/ناسازگار **fail-fast** باشد — یعنی هرگز یک
   `Config` نیمه‌معتبر برنگرداند
3. یکپارچگی ارجاعی را بررسی کند: هر `policy` که به یک role یا operation
   ناموجود اشاره کند، باید در همان لحظه‌ی بارگذاری رد شود، نه در زمان اجرا

**تصمیم فرمت:** به‌جای YAML از JSON استفاده شد (به ADR-004 مراجعه کنید).

## Acceptance Criteria
- [x] **AC-1:** یک config کاملاً سالم و بدون تناقض باید بارگذاری و validate شود
- [x] **AC-2:** دو operation با نام تکراری رد شود
- [x] **AC-3:** operation با command خالی رد شود
- [x] **AC-4:** policy که به role ناموجود اشاره کند رد شود
- [x] **AC-5:** policy که به operation ناموجود اشاره کند رد شود
- [x] **AC-6:** پارامتر از نوع int نباید pattern داشته باشد، و پارامتر از نوع
  string نباید min/max داشته باشد؛ min نباید از max بزرگ‌تر باشد؛ نوع
  ناشناخته رد شود
- [x] **AC-7:** `LoadConfig` باید فایل‌های JSON سالم را از دیسک بخواند
- [x] **AC-8:** `LoadConfig` باید در صورت نبود پوشه، خطا برگرداند (نه panic)
- [x] **AC-9:** `LoadConfig` باید در صورت JSON نامعتبر، خطا برگرداند
- [x] **AC-10:** اگر نتیجه‌ی نهایی از `ValidateConfig` رد شود، `LoadConfig`
  باید `nil, error` برگرداند، نه یک Config ناقص
- [x] **AC-11:** چند فایل JSON در یک پوشه باید با هم merge شوند
- [x] فایل‌های واقعی `configs/operations`, `configs/roles`, `configs/policies`
  باید خودشان با موفقیت بارگذاری و validate شوند (تست رگرسیون دائمی)

## Related Tests
- TEST-CONFIG-001 تا TEST-CONFIG-016
  (`internal/config/validation_test.go`, `internal/config/loader_test.go`)

## Related ADRs
- ADR-004 (انتخاب JSON به‌جای YAML برای M3)
