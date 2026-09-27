# شواهد M5 — Policy Engine

تاریخ بررسی: ۲۰۲۶-۰۹-۲۷. محیط اجرا: WSL، `go version go1.22.2 linux/amd64`.

## وضعیت اولیه و اصلاحات
- نسخه‌ی باز در این چت در `C:\Users\abolf\OneDrive\Documents\safeterm` بود.
  فایل policy به‌اشتباه داخل `internal/agent` و فایل تست صفر بایت بود.
- نسخه‌ی جداگانه‌ی `/home/abolfazl/safeterm` شامل انتقال صحیح فایل و ۷ تست
  Policy بود. تست‌های موجود از آن نسخه به پروژه‌ی این چت منتقل و تکمیل شدند؛
  نسخه‌ی جداگانه‌ی WSL تغییر داده نشد.
- بررسی پایه پیش از اصلاحات به‌علت فایل تست خالی و پایان خط CRLF شکست خورد.
  درخواست صریح مالک برای رفع کمبودهای M5 مبنای تعمیر این وضعیت بود.
- پیاده‌سازی Policy حفظ شد و کنترل nil برای درخواست و config اضافه شد.
  Core و چهار interface آن از نظر رفتار و امضا تغییر نکردند.
- REQ-005 و ADR-006 اضافه شدند؛ test matrix، README و توضیحات مرز احراز
  هویت اصلاح شدند. پایان خط Go و Makefile با `.gitattributes` تثبیت شد.
- `fmt-check` اکنون خطای اجرای gofmt را نیز برمی‌گرداند؛ نسخه‌ی قبلی می‌توانست
  در نبود خروجی stdout، شکست خود ابزار را پنهان کند. CI شاخه‌های `codex/**` را نیز اجرا می‌کند.

## نتیجه‌ی بررسی
[خروجی کامل و بدون خلاصه‌سازی make verify](make-verify.log):

```text
SafeTerm Verification: PASS
```

هر ۱۴ تست واحد Policy و ۳ تست یکپارچه‌ی M5 (با ۱۴ سناریوی یکپارچه)
موفق شدند. پوشش statementهای `internal/policy` برابر ۱۰۰٪ است.
تمام تست‌های مراحل قبل نیز موفق بودند. نگاشت دقیق معیارهای پذیرش و تست‌ها
در [test matrix](../../docs/testing/test-matrix.md) ثبت شده است.

[بررسی منفی fmt-check](fmt-check-negative.log) نیز تأیید کرد که gofmt ساختگی
با خروجی خالی و exit code ناموفق، باعث شکست هدف make می‌شود.

## بازتولید اجرا در WSL
به‌دلیل تفاوت مالکیت فایل‌ها روی درایو ویندوز، Git در WSL ابتدا خطای
`detected dubious ownership` داد. برای همان اجرای بررسی، فقط مسیر این پروژه
به Git معرفی شد؛ تنظیمات global تغییر نکرد و VCS stamping غیرفعال نشد:

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=safe.directory \
GIT_CONFIG_VALUE_0=/mnt/c/Users/abolf/OneDrive/Documents/safeterm \
make verify
```

## محدوده و انتشار
این شواهد برای تکمیل محلی M5 است. احراز هویت، Translation واقعی و Executor
واقعی در این تغییر پیاده‌سازی نشده‌اند. تست‌های M4 برای آزمون مستقل Validator
placeholder خود را نگه داشته‌اند؛ تست‌های M5 از Policy واقعی استفاده می‌کنند.

شاخه‌ی کار با دستور زیر ساخته شد:

```sh
git switch -c codex/m5-policy-completion
```

ادغام PR و ثبت تگ M5 پس از بازبینی مالک انجام می‌شود؛ PASS محلی به‌تنهایی
شاهد ادغام یا انتشار نیست. وضعیت CI باید در PR بررسی شود.
