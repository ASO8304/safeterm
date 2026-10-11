from collections.abc import AsyncIterator

from ariadne import SubscriptionType

subscription = SubscriptionType()

@subscription.source("webEvent")
async def source_web_event(_, info, topic: str) -> AsyncIterator[dict]:
    async for event in info.context["web_broker"].subscribe(topic):
        yield event

@subscription.field("webEvent")
def resolve_web_event(event, *_, **__):
    return event
