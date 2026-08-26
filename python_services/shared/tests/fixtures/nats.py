from nats.aio.msg import Msg
from nats.js.api import KeyValueConfig
from typing import Any
from typing import AsyncGenerator
from nats.js import JetStreamContext
import json
import pytest_asyncio
import nats


@pytest_asyncio.fixture
async def js_context(
    nats_url: str,
) -> AsyncGenerator[tuple[Any, JetStreamContext], None]:
    nc = await nats.connect(nats_url)  # type: ignore[import-untyped]
    js = nc.jetstream()
    try:
        await js.delete_stream("videos")
    except Exception:
        pass
    await js.add_stream(
        name="videos",
        subjects=["jobs.video.scene-split", "jobs.video.chunks"],
    )
    await js.create_key_value(config=KeyValueConfig(bucket="job-milestones"))
    yield nc, js
    await nc.close()


@pytest_asyncio.fixture
async def nats_video_chunks_subscriber(
    js_context: tuple[Any, JetStreamContext],
) -> AsyncGenerator[list[Any], None]:
    nc, js = js_context
    received = []

    async def handler(msg: Msg) -> None:
        received.append(json.loads(msg.data.decode()))

    sub = await nc.subscribe("jobs.video.chunks", cb=handler)
    yield received
    await sub.unsubscribe()
