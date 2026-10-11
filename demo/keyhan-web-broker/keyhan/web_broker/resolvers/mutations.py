from datetime import datetime, timezone
from uuid import uuid4

from ariadne import MutationType

mutation = MutationType()

@mutation.field("publishWebEvent")
async def resolve_publish_event(_, info, topic: str, payload: dict):
    if not topic or len(topic) > 80 or any(
        c not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._:-"
        for c in topic
    ):
        raise ValueError("invalid topic")
    event = {
        "id": str(uuid4()),
        "topic": topic,
        "payload": payload,
        "createdAt": datetime.now(timezone.utc).isoformat(),
    }
    await info.context["web_broker"].publish(topic, event)
    return event
