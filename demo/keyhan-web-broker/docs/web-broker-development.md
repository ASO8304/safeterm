# Web Broker Development Guide

## Repository shape

The demo mirrors the backend layout supplied by the backend lead:

    config/gql/schema.graphql
    keyhan/web_broker/
      app.py
      broker.py
      resolvers/
        queries.py
        mutations.py
        subscriptions.py

Add a new GraphQL operation to config/gql/schema.graphql, then implement its
resolver in the matching resolver module. Subscription fields require both a
source generator and a field resolver.

## Event flow

    Mutation or service
        -> broker.publish(topic, event)
        -> broker.subscribe(topic)
        -> Ariadne Subscription source
        -> GraphQL WebSocket client

The source must use async for and must not perform blocking work on the event
loop. A subscriber is registered when the subscription starts and removed when
the client completes or disconnects.

## Topic contract

Topic names are bounded to 80 characters and may contain letters, digits,
period, underscore, colon and hyphen. Use stable names such as job:123 or
notifications:user:42. Do not accept a topic as proof of authorization:
resolve the current user from the WebSocket context and check ownership or
role permissions before subscribing.

Events contain an opaque id, topic, payload and createdAt timestamp. Payloads
should be small and must not contain passwords, access tokens or unredacted
personal data.

## Gunicorn and Nginx

The default process configuration is one UvicornWorker. Nginx must preserve the
WebSocket upgrade headers and use a long read timeout. The GraphQL mount uses
the trailing slash /graphql/.

## Testing

Run the complete demo checks with:

    ./scripts/create-demo.sh

The tests cover the health query, publishing mutation, delivery to a
subscriber, topic isolation and bounded in-process broker behavior.

Before changing the worker count, add an integration test using the selected
multi-process broker. A passing MemoryWebBroker unit test does not prove
delivery across workers.

## Moving to Redis

Install the optional client and set WEB_BROKER_REDIS_URL:

    .venv/bin/pip install 'redis>=5,<6'
    WEB_BROKER_REDIS_URL=redis://127.0.0.1:6379/0 ./scripts/run-demo.sh

Redis Pub/Sub is still non-durable. Use Redis Streams or a durable event system
if reconnecting clients must replay missed events.
