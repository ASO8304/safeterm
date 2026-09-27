# شواهد M6 — Translation Layer

تاریخ: ۲۰۲۶-۰۹-۲۷. محیط: WSL، Go 1.22.2 روی Linux/amd64.

## پایه و محدوده
پایه‌ی شاخه، commit ادغام‌شده‌ی M5 یعنی `03b1fae` از `origin/main` است.
[بررسی پایه پیش از تغییر](baseline-verify.log) با `SafeTerm Verification: PASS`
تمام شد. نسخه‌ی جداگانه‌ی `/home/abolfazl/safeterm` و stash پشتیبان آن تغییر نکردند.

بررسی مخزن نشان داد برخلاف متن پرامپت، system_info دارای `args_template: ["-a"]`
است. همین تعریف حفظ شد و قالب خالی با fixture مستقل تست شد. همچنین تگ M5
هنوز در تگ‌های دریافت‌شده وجود نداشت؛ README وضعیت ادغام و تگ را جدا گزارش می‌کند.

## نیازمندی و تصمیم
- REQ-006: یازده معیار پذیرش برای ترجمه، رد ورودی ناقص، استقلال داده‌ها و توقف pipeline.
- ADR-007: placeholder فقط یک توکن کامل است؛ قالب خراب هنگام ساخت رد می‌شود؛
  نبود مقدار حتی برای placeholder اختیاری خطاست؛ آرگومان‌ها join یا split نمی‌شوند.
- `translation.New` به‌دلیل خطاهای قالب، خروجی error دارد؛ interface مراحل و رفتار
  Core تغییر نکرده‌اند. literalها و فرمان از config مورد اعتماد گرفته می‌شوند.
- IdentityTranslator در تست‌های مستقل قبلی حفظ شده و مسیر یکپارچه‌ی M6
  از Validator، Policy و Translator واقعی و Executor صرفاً آزمایشی استفاده می‌کند.

## تست و نتیجه
[خروجی کامل make verify](make-verify.log) بدون خلاصه‌سازی ذخیره شده است.

```text
SafeTerm Verification: PASS
```

- TEST-TRANS-001 تا TEST-TRANS-009، به‌ترتیب AC-1 تا AC-9 را پوشش می‌دهند.
- TEST-INTEG-007: دریافت دقیق فرمان و argv توسط Executor آزمایشی (AC-10).
- TEST-INTEG-008: توقف ورودی نامعتبر و نقش غیرمجاز پیش از Translation (AC-11).
- TEST-INTEG-009: عبور پارامتر اختیاریِ غایب از Validation و رد آن در Translation
  بدون فراخوانی Executor (AC-6 و AC-11).
- ۹ تست واحد و ۳ تست یکپارچه‌ی جدید موفق شدند؛ تست‌های قبلی نیز موفق بودند.
  پوشش statementهای پکیج translation برابر ۱۰۰٪ است.

## بازتولید اجرا
در checkout ویندوز که از WSL اجرا می‌شود، اعتماد Git فقط برای همان فراخوانی
و همان مسیر تنظیم شد؛ هیچ تنظیم global یا وابستگی خارجی اضافه نشد:

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=safe.directory \
GIT_CONFIG_VALUE_0=/mnt/c/Users/abolf/OneDrive/Documents/safeterm \
make verify
```

شاخه با دستور زیر از پایه‌ی M5 ساخته شد:

```sh
git switch -c codex/m6-translation origin/main
```

## محدودیت و بازبینی
Translate فرض می‌کند Validation و Policy با همان config اجرا شده‌اند و بررسی
نوع یا مجوز را تکرار نمی‌کند. Config منبع مورد اعتماد است؛ تفسیر گزینه‌های
اختصاصی هر executable و اجرای واقعی process جزو این milestone نیست.
احراز هویت، M7 و مراحل بعدی شروع نشده‌اند. نتیجه‌ی CI در PR بررسی می‌شود؛
ادغام و تگ M6 پس از بازبینی مالک انجام می‌شوند.
