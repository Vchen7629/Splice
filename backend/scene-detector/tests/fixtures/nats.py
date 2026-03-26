from typing import Any
from typing import Generator
from typing import AsyncGenerator
from testcontainers.nats import NatsContainer
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
async def nats_video_chunks_subscriber(nats_url, monkeypatch) -> AsyncGenerator[list[Any], None]:
    monkeypatch.setattr("src.nats.publisher.settings.VIDEO_CHUNKS_SUBJECT", "jobs.video.chunks")
    nc = await nats.connect(nats_url)
    js = nc.jetstream()                                                                                                                        
    received = []

    await js.add_stream(name="chunks", subjects=["jobs.video.chunks"])
                                                                                                                                                            
    async def handler(msg):                                                                                                                                  
        received.append(json.loads(msg.data.decode()))
                                                                                                                                                            
    sub = await nc.subscribe("jobs.video.chunks", cb=handler)
    yield received
    await sub.unsubscribe()                                                                                                                                  
    await nc.close()