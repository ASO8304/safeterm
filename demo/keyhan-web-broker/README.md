# Keyhan Web Broker Demo

این پروژه خارج از repository اصلی SafeTerm ساخته شده و ساختار پیشنهادی بک‌اند را تقلید می‌کند: schema در config/gql، resolverها در keyhan/web_broker/resolvers، بروکر داخلی، Gunicorn با UvicornWorker و Nginx با پشتیبانی WebSocket.

مستندات تصمیم معماری در docs/adr/ADR-001-web-broker-architecture.md و راهنمای
توسعه در docs/web-broker-development.md قرار دارد.

## ساخت و تست

    ./scripts/create-demo.sh

این اسکریپت virtualenv می‌سازد، dependencyها را نصب می‌کند و تست‌ها را اجرا می‌کند.

## اجرای محلی

    ./scripts/run-demo.sh

GraphQL روی http://127.0.0.1:8000/graphql/ اجرا می‌شود. حالت پیش‌فرض MemoryWebBroker است و Gunicorn عمداً با یک worker اجرا می‌شود؛ این ساده‌ترین حالت برای یک اتصال همزمان است.

## چه زمانی Redis لازم می‌شود؟

Redis برای یک worker و تعداد کم اتصال لازم نیست. فقط وقتی لازم می‌شود که تعداد workerها بیشتر از یک باشد، چند instance از سرویس اجرا شود، یا پیام‌ها باید بین processها مشترک باشند:

برای استفاده از حالت Redis باید dependency اختیاری redis را جداگانه نصب کنید:

    .venv/bin/pip install 'redis>=5,<6'
    WEB_BROKER_REDIS_URL=redis://127.0.0.1:6379/0 ./scripts/run-demo.sh

برای اتصال پشت Nginx از nginx/keyhan-web-broker.conf استفاده کنید.
