# ADR-001: Lightweight Web Broker for GraphQL Subscriptions

Status: Accepted

Date: 2026-10-11

## Context

The backend exposes GraphQL through Ariadne. The web product needs a small
real-time channel for a very low number of simultaneous connections. The
deployment uses Gunicorn behind Nginx and does not currently require multiple
backend instances or durable event delivery.

## Decision

The first implementation uses an in-process asynchronous broker:

- Ariadne owns GraphQL parsing and the graphql-transport-ws WebSocket protocol.
- Subscription.source consumes an async generator from MemoryWebBroker.
- Mutations and backend services publish events through the broker interface.
- Gunicorn runs one UvicornWorker.
- Nginx forwards HTTP and WebSocket upgrade requests to the same process.
- The broker uses bounded per-topic queues to apply backpressure.
- The default deployment does not install or require Redis.

The broker is an application notification channel, not a durable queue. Events
may be lost when the process restarts or when no subscriber is connected.

## Consequences

This design has the lowest operational and resource overhead for the current
product. It keeps deployment simple and avoids an external service. A second
Gunicorn worker or a second backend instance must not be enabled while using
MemoryWebBroker, because each process has a separate topic registry.

Redis becomes an optional adapter when one of these requirements appears:

- more than one worker or backend instance;
- publishers and subscribers in different processes;
- cross-instance event fan-out;
- a requirement to retain or replay events.

Adding Redis must not change the GraphQL schema or resolver contract. Only the
broker implementation and deployment configuration should change.

## Rejected alternatives

Redis was not selected as the default because it adds a process, dependency,
configuration, monitoring and failure mode that are unnecessary for one
connection. A local asyncio.Queue per resolver was rejected because it would
duplicate lifecycle and cleanup logic across applications.

## Operational rules

1. Run one Gunicorn worker with MemoryWebBroker.
2. Keep queue capacity bounded.
3. Validate topic names before publishing.
4. Check user authorization before subscribing to a topic.
5. Always remove a subscriber in the generator's finally block.
6. Never place secrets in event payloads or logs.
