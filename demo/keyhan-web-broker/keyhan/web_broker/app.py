from pathlib import Path

from ariadne import ScalarType, load_schema_from_path, make_executable_schema
from ariadne.asgi import GraphQL
from ariadne.asgi.handlers import GraphQLTransportWSHandler
from starlette.applications import Starlette

from .broker import create_broker
from .resolvers import mutation, query, subscription

ROOT = Path(__file__).resolve().parents[2]
json_scalar = ScalarType("JSON")
json_scalar.set_serializer(lambda value: value)
type_defs = load_schema_from_path(ROOT / "config" / "gql" / "schema.graphql")
schema = make_executable_schema(type_defs, [json_scalar, query, mutation, subscription])
broker = create_broker()

def context_value(request, data=None):
    return {"web_broker": broker, "request": request}

async def startup():
    await broker.startup()

async def shutdown():
    await broker.shutdown()

graphql = GraphQL(
    schema,
    context_value=context_value,
    websocket_handler=GraphQLTransportWSHandler(),
)
app = Starlette(on_startup=[startup], on_shutdown=[shutdown])
app.mount("/graphql", graphql)
