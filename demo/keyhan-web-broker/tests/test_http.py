import asyncio

import httpx

from keyhan.web_broker.app import app

def test_health_and_publish():
    async def scenario():
        transport = httpx.ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
            health = await client.post("/graphql/", json={"query": "{ health }"})
            assert health.json() == {"data": {"health": "ok"}}
            result = await client.post(
                "/graphql/",
                json={"query": 'mutation { publishWebEvent(topic: "demo", payload: {status: "ready"}) { topic payload } }'},
            )
            assert result.status_code == 200
            assert result.json()["data"]["publishWebEvent"]["topic"] == "demo"

    asyncio.run(scenario())
