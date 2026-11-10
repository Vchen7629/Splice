import asyncio
import json
from contextlib import contextmanager
from typing import Any, AsyncGenerator, Generator
from unittest.mock import AsyncMock, MagicMock, patch

import nats
import pytest
import pytest_asyncio
from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from core import settings
from msgbus import JobContext, ProcessJobMessage


def milestone_entry(state: str, stage: str = "", revision: int = 1) -> MagicMock:
    """Fake job-milestones KV entry"""
    payload = {"state": state, "stage": stage} if stage else {"state": state}
    return MagicMock(value=json.dumps(payload).encode(), revision=revision)


def make_msg(
    job_id: str = "job-123",
    storage_url: str = "http://storage/video.mp4",
    source_resolution: str = "480p",
    target_resolution: str = "1080p",
) -> AsyncMock:
    """Mock NATS Msg carrying a valid ProcessJobMessage payload"""
    payload = ProcessJobMessage(
        job_id=job_id,
        operation="upscale",
        storage_url=storage_url,
        source_resolution=source_resolution,
        target_resolution=target_resolution,
    )
    msg = AsyncMock(spec=Msg)
    msg.data = payload.model_dump_json().encode()
    return msg


@pytest.fixture
def mock_logger() -> MagicMock:
    return MagicMock(spec=BoundLogger)


@pytest.fixture
def processed_kv() -> AsyncMock:
    """Empty upscale-processed KV, so no job counts as already processed"""
    kv = AsyncMock(spec=KeyValue)
    kv.get.side_effect = KeyNotFoundError()
    return kv


@pytest.fixture
def milestone_kv() -> AsyncMock:
    """job-milestones KV holding a PROCESSING/upload entry for any job"""
    kv = AsyncMock(spec=KeyValue)
    kv.get.return_value = milestone_entry("PROCESSING", "upload")
    return kv


@pytest.fixture
def job_ctx(milestone_kv: AsyncMock) -> JobContext:
    """JobContext with mocked NATS clients"""
    return JobContext(
        AsyncMock(spec=NATSClient), AsyncMock(spec=JetStreamContext), milestone_kv
    )


@contextmanager
def _patch_main_startup(nc: Any, js: Any) -> Generator[None, None, None]:
    """Patches main's storage health check, health server and NATS connection"""
    with (
        patch("main.check_storage_health"),
        patch("main.start_http_server"),
        patch("main.nats_connect", return_value=(nc, js)),
    ):
        yield


@pytest.fixture
def service_patches() -> Generator[tuple[MagicMock, MagicMock], None, None]:
    """run_service with startup fully mocked, yields the (nc, js) mocks"""
    nc = MagicMock(is_closed=False, drain=AsyncMock())
    js = MagicMock(
        find_stream_name_by_subject=AsyncMock(),
        create_key_value=AsyncMock(),
        key_value=AsyncMock(),
        subscribe=AsyncMock(),
    )
    with _patch_main_startup(nc, js):
        yield nc, js


@pytest_asyncio.fixture
async def js_context(
    nats_url: str,
) -> AsyncGenerator[tuple[NATSClient, JetStreamContext], None]:
    """Real NATS connection with a stream for the service's subjects and a job-milestones KV"""
    nc = await nats.connect(nats_url)
    js = nc.jetstream()
    try:
        await js.delete_stream("videos")
    except Exception:
        pass
    await js.add_stream(
        name="videos", subjects=[settings.SUB_SUBJECT, settings.PUB_SUBJECT]
    )
    await js.create_key_value(config=KeyValueConfig(bucket="job-milestones"))
    yield nc, js
    await nc.close()


@pytest_asyncio.fixture
async def patched_start_service(
    js_context: tuple[NATSClient, JetStreamContext],
    milestone_kv: AsyncMock,
    processed_kv: AsyncMock,
) -> AsyncGenerator[tuple[NATSClient, JetStreamContext], None]:
    """run_service against real NATS, with storage/health mocked and both KVs faked"""
    nc, js = js_context
    with (
        _patch_main_startup(nc, js),
        patch("main.kv_connect", return_value=milestone_kv),
        patch("main.create_kv", return_value=processed_kv),
    ):
        yield nc, js


@pytest_asyncio.fixture
async def complete_subscriber(
    js_context: tuple[NATSClient, JetStreamContext],
) -> AsyncGenerator[tuple[list[dict[str, Any]], asyncio.Event], None]:
    """Collects messages on settings.PUB_SUBJECT, the event is set on the first one"""
    nc, _ = js_context
    received: list[dict[str, Any]] = []
    first = asyncio.Event()

    async def handler(msg: Msg) -> None:
        received.append(json.loads(msg.data))
        first.set()

    sub = await nc.subscribe(settings.PUB_SUBJECT, cb=handler)
    yield received, first
    await sub.unsubscribe()


@pytest.fixture
def spy_drain(
    js_context: tuple[NATSClient, JetStreamContext],
) -> tuple[Any, list[bool]]:
    """Replaces nc.drain with a no-op spy"""
    nc, _ = js_context
    called: list[bool] = []

    async def _spy() -> None:
        called.append(True)

    nc.drain = _spy
    return nc, called
