from typing import Any
from typing import AsyncGenerator
from nats.js import JetStreamContext
from nats.aio.msg import Msg
from src.core.settings import settings
import json
import pytest_asyncio


@pytest_asyncio.fixture
async def nats_video_chunks_subscriber(
    js_context: tuple[Any, JetStreamContext],
) -> AsyncGenerator[list[Any], None]:
    nc, js = js_context
    received = []

    async def handler(msg: Msg) -> None:
        received.append(json.loads(msg.data.decode()))

    sub = await nc.subscribe(settings.PUB_SUBJECT, cb=handler)
    yield received
    await sub.unsubscribe()
