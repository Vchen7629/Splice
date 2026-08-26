from typing import Any, AsyncGenerator
from unittest.mock import patch, AsyncMock, MagicMock
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from src.core.settings import settings
import nats  # type: ignore[import-untyped]
import pytest
import pytest_asyncio


@pytest_asyncio.fixture
async def js_context(
    nats_url: str,
) -> AsyncGenerator[tuple[Any, JetStreamContext], None]:
    nc = await nats.connect(nats_url)
    js = nc.jetstream()
    try:
        await js.delete_stream("videos")
    except Exception:
        pass
    await js.add_stream(
        name="videos",
        subjects=[settings.SUB_SUBJECT, settings.PUB_SUBJECT],
    )
    await js.create_key_value(config=KeyValueConfig(bucket="job-milestones"))
    yield nc, js
    await nc.close()


@pytest_asyncio.fixture
async def patched_start_service(
    js_context: tuple[Any, JetStreamContext],
) -> AsyncGenerator[tuple[Any, JetStreamContext], None]:
    """Yields (nc, js) with check_storage_health, start_health_server, and nats_connect patched"""
    nc, js = js_context

    mock_kv = MagicMock(spec=KeyValue)
    mock_kv.get = AsyncMock(side_effect=KeyNotFoundError())
    mock_kv.put = AsyncMock()

    with (
        patch("src.service.check_storage_health"),
        patch("src.service.start_health_server"),
        patch("src.service.nats_connect", return_value=(nc, js)),
        patch("src.service.connect_kv", new_callable=AsyncMock),
        patch("src.service.create_kv", return_value=mock_kv),
    ):
        yield nc, js


@pytest.fixture
def service_patches(mock_nats: tuple[MagicMock, MagicMock]) -> Any:
    """Patches check_storage_health, start_health_server, and nats_connect with mocked nats"""
    mock_nc, mock_js = mock_nats
    with (
        patch("src.service.check_storage_health"),
        patch("src.service.start_health_server"),
        patch("src.service.nats_connect", return_value=(mock_nc, mock_js)),
    ):
        yield mock_nc, mock_js


@pytest.fixture
def spy_drain(js_context: tuple[Any, JetStreamContext]) -> tuple[Any, list[bool]]:
    """Replaces nc.drain with a no-op spy"""
    nc, _ = js_context
    called: list[bool] = []

    async def _spy() -> None:
        called.append(True)

    nc.drain = _spy
    return nc, called
