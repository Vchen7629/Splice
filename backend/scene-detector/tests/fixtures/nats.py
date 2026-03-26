from typing import Any
from typing import Generator
from typing import AsyncGenerator
from testcontainers.nats import NatsContainer
from src.core.settings import settings
import nats
import json
import pytest
import pytest_asyncio


@pytest.fixture(scope="session")
def nats_url() -> Generator[str, None]:
    """Starts a nats container and returns url"""
    with NatsContainer(jetstream=True) as container:
        yield container.nats_uri()


@pytest_asyncio.fixture
async def js_context(nats_url) -> AsyncGenerator[tuple, None]:
    nc = await nats.connect(nats_url)
    js = nc.jetstream()
    try:
        await js.delete_stream("videos")
    except Exception:
        pass
    await js.add_stream(
        name="videos",
        subjects=[settings.SCENE_SPLIT_SUBJECT, settings.VIDEO_CHUNKS_SUBJECT],
    )
    yield nc, js
    await nc.close()


@pytest_asyncio.fixture
async def nats_video_chunks_subscriber(
    js_context, monkeypatch
) -> AsyncGenerator[list[Any], None]:
    monkeypatch.setattr(
        "src.nats.publisher.settings.VIDEO_CHUNKS_SUBJECT",
        settings.VIDEO_CHUNKS_SUBJECT,
    )
    nc, js = js_context
    received = []

    async def handler(msg):
        received.append(json.loads(msg.data.decode()))

    sub = await nc.subscribe(settings.VIDEO_CHUNKS_SUBJECT, cb=handler)
    yield received
    await sub.unsubscribe()
