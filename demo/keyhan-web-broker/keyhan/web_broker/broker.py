from __future__ import annotations

import asyncio
import json
import os
from collections.abc import AsyncIterator


class MemoryWebBroker:
    """Single-process demo broker; use RedisWebBroker for multiple workers."""

    def __init__(self) -> None:
        self._topics: dict[str, set[asyncio.Queue[dict]]] = {}
        self._lock = asyncio.Lock()

    async def publish(self, topic: str, event: dict) -> None:
        async with self._lock:
            queues = tuple(self._topics.get(topic, ()))
        for queue in queues:
            await queue.put(event)

    async def subscribe(self, topic: str) -> AsyncIterator[dict]:
        queue: asyncio.Queue[dict] = asyncio.Queue(maxsize=100)
        async with self._lock:
            self._topics.setdefault(topic, set()).add(queue)
        try:
            while True:
                yield await queue.get()
        finally:
            async with self._lock:
                queues = self._topics.get(topic)
                if queues:
                    queues.discard(queue)
                    if not queues:
                        self._topics.pop(topic, None)

    async def startup(self) -> None:
        return None

    async def shutdown(self) -> None:
        return None


class RedisWebBroker:
    def __init__(self, url: str) -> None:
        from redis.asyncio import Redis
        self.redis = Redis.from_url(url, encoding="utf-8", decode_responses=True)

    async def startup(self) -> None:
        await self.redis.ping()

    async def shutdown(self) -> None:
        await self.redis.aclose()

    async def publish(self, topic: str, event: dict) -> None:
        await self.redis.publish(self._channel(topic), json.dumps(event))

    async def subscribe(self, topic: str) -> AsyncIterator[dict]:
        pubsub = self.redis.pubsub()
        await pubsub.subscribe(self._channel(topic))
        try:
            async for message in pubsub.listen():
                if message["type"] == "message":
                    yield json.loads(message["data"])
        finally:
            await pubsub.unsubscribe(self._channel(topic))
            await pubsub.aclose()

    @staticmethod
    def _channel(topic: str) -> str:
        return f"keyhan:web:{topic}"


def create_broker():
    redis_url = os.getenv("WEB_BROKER_REDIS_URL")
    return RedisWebBroker(redis_url) if redis_url else MemoryWebBroker()
