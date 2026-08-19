from typing import Any, AsyncGenerator, Generator
from unittest.mock import patch, AsyncMock, MagicMock
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from testcontainers.nats import NatsContainer
from src.core.settings import settings
import nats  # type: ignore[import-untyped]
import pytest
import pytest_asyncio


@pytest.fixture
def nats_msg_patches() -> Generator[dict[str, Any], Any, None]:
    """Patches all external dependencies used by process_msg / _finalize_job."""
    with (
        patch(
            "src.processing.nats_msg.check_already_processed", new_callable=AsyncMock
        ) as mock_check,
        patch(
            "src.processing.nats_msg.update_job_status", new_callable=AsyncMock
        ) as mock_update_status,
        patch(
            "src.processing.nats_msg.fetch_video", return_value="/tmp/job-123/video.mp4"
        ) as mock_fetch,
        patch("src.processing.nats_msg.select_model") as mock_select,
        patch("src.processing.nats_msg.video_upscale") as mock_upscale,
        patch("src.processing.nats_msg.video_downscale") as mock_downscale,
        patch("src.processing.nats_msg.upload_video") as mock_upload,
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock) as mock_pub,
        patch("src.processing.nats_msg.shutil.rmtree") as mock_rmtree,
        patch("src.processing.nats_msg.os.makedirs") as _,
        patch(
            "src.processing.nats_msg.asyncio.to_thread",
            side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs),
        ) as _,
    ):
        mock_check.return_value = False
        yield {
            "check": mock_check,
            "update_status": mock_update_status,
            "fetch": mock_fetch,
            "select": mock_select,
            "upscale": mock_upscale,
            "downscale": mock_downscale,
            "upload": mock_upload,
            "pub": mock_pub,
            "rmtree": mock_rmtree,
        }


@pytest.fixture
def mock_nats() -> tuple[MagicMock, MagicMock]:
    mock_js = MagicMock()
    mock_js.find_stream_name_by_subject = AsyncMock()
    mock_js.create_key_value = AsyncMock()
    mock_js.key_value = AsyncMock()
    mock_nc = MagicMock()
    mock_nc.drain = AsyncMock()
    return mock_nc, mock_js


@pytest.fixture(scope="session")
def nats_url() -> Any:
    with NatsContainer(jetstream=True) as container:
        yield container.nats_uri()


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
    await js.create_key_value(config=KeyValueConfig(bucket="job-status"))
    yield nc, js
    await nc.close()


@pytest_asyncio.fixture
async def patched_start_service(
    js_context: tuple[Any, JetStreamContext],
) -> AsyncGenerator[tuple[Any, JetStreamContext], None]:
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
def spy_drain(js_context: tuple[Any, JetStreamContext]) -> tuple[Any, list[bool]]:
    nc, _ = js_context
    called: list[bool] = []

    async def _spy() -> None:
        called.append(True)

    nc.drain = _spy
    return nc, called
