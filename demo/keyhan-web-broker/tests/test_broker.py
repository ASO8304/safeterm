import asyncio

from keyhan.web_broker.broker import MemoryWebBroker

async def receive_one(broker, result):
    async for event in broker.subscribe("orders"):
        result.append(event)
        return

def test_publish_reaches_subscriber_and_topics_are_isolated():
    async def scenario():
        broker = MemoryWebBroker()
        received = []
        task = asyncio.create_task(receive_one(broker, received))
        await asyncio.sleep(0)
        await broker.publish("other", {"id": "wrong"})
        await broker.publish("orders", {"id": "right"})
        await asyncio.wait_for(task, timeout=1)
        assert received == [{"id": "right"}]
    asyncio.run(scenario())
