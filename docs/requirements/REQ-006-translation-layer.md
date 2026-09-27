# REQ-006: ترجمه‌ی عملیات مجاز به دستور ثابت و آرگومان‌های جداگانه

**Source:** M6 — Translation Layer، مثال trace در پرامپت ادامه‌ی پروژه و قرارداد `agent.Translator`
**Priority:** Critical
**Status:** Done

## Description
پکیج `internal/translation` درخواست عبورکرده از Validation و Policy را به
نام دستور ثابت در config و `[]string` آرگومان‌ها تبدیل می‌کند. این پکیج جایگزین
`IdentityTranslator` در مسیر یکپارچه‌ی M6 است؛ هیچ shell یا process اجرا نمی‌کند.

هر توکن ArgsTemplate یا literal است یا یک placeholder کامل `{name}`.
تصمیم درباره‌ی دستور زبان محدود قالب، بررسی هنگام ساخت و پارامتر اختیاری
در ADR-007 ثبت شده است. بررسی نوع، بازه و pattern همچنان وظیفه‌ی Validator
و بررسی مجوز همچنان وظیفه‌ی Policy است؛ فراخوانی مستقیم Translate جایگزین این مراحل نیست.

## Acceptance Criteria
- [x] **AC-1:** trace با port=999 و host=192.168.10.20 به دستور tcpdump و دقیقاً
  `["-i", "any", "port", "999", "and", "host", "192.168.10.20"]` ترجمه شود.
- [x] **AC-2:** دستور فقط از config و بدون تغییر گرفته شود. ترتیب literalها و
  جایگزینی‌ها حفظ شود؛ فاصله یا رشته‌ی خالی در یک مقدار به همان یک آرگومان تبدیل شود.
  placeholder تکراری مجاز باشد و هیچ join، split یا تفسیر shell انجام نشود.
- [x] **AC-3:** قالب nil یا خالی، آرگومان خالی تولید کند؛ system_info واقعی طبق
  config مخزن به uname با آرگومان `-a` ترجمه شود.
- [x] **AC-4:** سازنده Config nil، Config نامعتبر و نام پارامتر تکراری را با خطا
  و بدون Translator قابل‌استفاده رد کند.
- [x] **AC-5:** placeholder بدون تعریف، آکولاد ناقص/تو‌در‌تو/خالی و placeholder
  ترکیبی با literal، هنگام ساخت Translator با خطای مشخص رد شوند.
- [x] **AC-6:** نبود مقدار هر پارامتر استفاده‌شده در قالب، حتی پارامتر اختیاری،
  خطا بدهد؛ خروجی خطا همیشه دستور خالی و args=nil باشد، نه خروجی نیمه‌ساخته.
- [x] **AC-7:** درخواست nil، عملیات ناشناخته، نام خام executable و Translator
  با مقدار صفر بدون panic رد شوند؛ هیچ fallback به req.Command وجود نداشته باشد.
- [x] **AC-8:** سازنده، config را تغییر ندهد و snapshot مستقلی از فرمان و قالب
  بسازد؛ Translate درخواست را تغییر ندهد و خروجی هر فراخوانی مستقل باشد.
- [x] **AC-9:** فقط توکن‌های قالب در خروجی ظاهر شوند؛ پارامتر تعریف‌شده ولی
  استفاده‌نشده خودکار به آرگومان‌ها اضافه نشود.
- [x] **AC-10:** در Core با Config، Validator، Policy و Translator واقعی، Executor
  آزمایشی دقیقاً فرمان و آرگومان نهایی trace و system_info را دریافت کند.
- [x] **AC-11:** ورودی نامعتبر و نقش غیرمجاز پیش از Translation متوقف شوند؛
  خطای Translation با نتیجه rejected و مرحله translation، Executor را فراخوانی نکند.

## Related Tests
- TEST-TRANS-001 تا TEST-TRANS-009 (`internal/translation/translator_test.go`)
- TEST-INTEG-007 تا TEST-INTEG-009 (`tests/integration/agent_translation_test.go`)

## Related ADRs
- ADR-003 (مراحل مستقل و توقف بعد از شکست)
- ADR-005 (اعتبارسنجی ورودی)
- ADR-007 (قالب توکن کامل و ترجمه بدون shell)
