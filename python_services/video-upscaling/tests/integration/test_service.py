from typing import Any
from unittest.mock import patch
from unittest.mock import AsyncMock
from nats.js import JetStreamContext
from shared_storage import queries
from src.service import start_service
from src.core.settings import settings
import json
import uuid
import pytest
import asyncio


def _make_payload(
    job_id: str,
    storage_url: str,
    source_resolution: str = "720p",
    target_resolution: str = "480p",
) -> bytes:
    return json.dumps(
        {
            "job_id": job_id,
            "storage_url": storage_url,
            "source_resolution": source_resolution,
            "target_resolution": target_resolution,
        }
    ).encode()


async def _run_service_until_processed(
    nc: Any,
    payload: bytes,
    queue_name: str,
    done: asyncio.Event | None = None,
    timeout: float = 30.0,
) -> None:
    with patch("src.service.settings.SUB_QUEUE_NAME", queue_name):
        nc.drain = AsyncMock()
        task = asyncio.create_task(start_service())
        await nc.publish(settings.SUB_SUBJECT, payload)
        if done is not None:
            await asyncio.wait_for(done.wait(), timeout=timeout)
            await asyncio.sleep(0.1)  # let NATS callbacks fire
        else:
            await asyncio.sleep(timeout)
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass


@pytest.mark.asyncio
async def test_full_job_publishes_upscale_complete_msg(
    patched_start_service: tuple[Any, JetStreamContext],
    uploaded_test_video: str,
    seaweedfs_url: str,
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Any,
) -> None:
    """Full flow: message published -> video fetched from SeaweedFS -> downscaled -> UpscaleCompleteMsg on downstream subject."""
    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))
    monkeypatch.setattr("processing.nats_msg.settings.BASE_STORAGE_URL", seaweedfs_url)

    nc, js = patched_start_service
    job_id = str(uuid.uuid4())
    received: list[Any] = []

    done = asyncio.Event()

    async def capture(msg: Any) -> None:
        received.append(json.loads(msg.data.decode()))
        done.set()

    sub = await nc.subscribe(settings.PUB_SUBJECT, cb=capture)

    await _run_service_until_processed(
        nc,
        _make_payload(
            job_id,
            uploaded_test_video,
            source_resolution="720p",
            target_resolution="480p",
        ),
        "test-full-flow-worker",
        done=done,
    )

    await sub.unsubscribe()
    assert any(m.get("job_id") == job_id for m in received)


@pytest.mark.asyncio
async def test_full_job_uploads_output_to_storage(
    patched_start_service: tuple[Any, JetStreamContext],
    uploaded_test_video: str,
    seaweedfs_url: str,
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Any,
) -> None:
    """After processing, output video is PUT to SeaweedFS at the expected URL."""
    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))
    monkeypatch.setattr("processing.nats_msg.settings.BASE_STORAGE_URL", seaweedfs_url)

    nc, js = patched_start_service
    job_id = str(uuid.uuid4())
    uploaded_urls: list[str] = []

    original_upload = __import__(
        "shared_storage.queries", fromlist=["upload_video"]
    ).upload_video
    done = asyncio.Event()

    def spy_upload(url: str, *args: Any, **kwargs: Any) -> str:
        uploaded_urls.append(url)
        result = original_upload(url, *args, **kwargs)
        done.set()
        return result

    with patch("processing.nats_msg.upload_video", side_effect=spy_upload):
        await _run_service_until_processed(
            nc,
            _make_payload(
                job_id,
                uploaded_test_video,
                source_resolution="720p",
                target_resolution="480p",
            ),
            "test-upload-worker",
            done=done,
        )

    expected_url = f"{seaweedfs_url}/{job_id}/output.mp4/processed"
    assert any(expected_url in url for url in uploaded_urls)


# ---------------------------------------------------------------------------
# startup failures
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_raises_when_pub_stream_not_found(
    patched_start_service: tuple[Any, JetStreamContext],
    monkeypatch: Any,
) -> None:
    nc, _ = patched_start_service
    monkeypatch.setattr("src.service.settings.PUB_SUBJECT", "nonexistent.subject.xyz")
    nc.drain = AsyncMock()

    with pytest.raises(RuntimeError, match="No stream found"):
        await start_service()


@pytest.mark.asyncio
async def test_raises_when_sub_stream_not_found(
    patched_start_service: tuple[Any, JetStreamContext],
    monkeypatch: Any,
) -> None:
    nc, _ = patched_start_service
    monkeypatch.setattr("src.service.settings.SUB_SUBJECT", "nonexistent.subject.xyz")
    nc.drain = AsyncMock()

    with pytest.raises(RuntimeError, match="No stream found"):
        await start_service()


# ---------------------------------------------------------------------------
# finally block – drain always called
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_drain_called_on_consumer_failure(
    patched_start_service: tuple[Any, JetStreamContext],
    spy_drain: tuple[Any, list[bool]],
) -> None:
    _, called = spy_drain

    with (
        patch("src.service.consumer", side_effect=RuntimeError("boom")),
        pytest.raises(RuntimeError),
    ):
        await start_service()

    assert called


@pytest.mark.asyncio
async def test_drain_called_on_cancellation(
    patched_start_service: tuple[Any, JetStreamContext],
    spy_drain: tuple[Any, list[bool]],
) -> None:
    _, called = spy_drain

    async def _hang(*_: Any, **__: Any) -> None:
        await asyncio.Event().wait()

    with patch("src.service.consumer", side_effect=_hang):
        task = asyncio.create_task(start_service())
        await asyncio.sleep(0.05)
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert called


# ---------------------------------------------------------------------------
# storage health check blocks startup
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_raises_before_nats_when_storage_unreachable(monkeypatch: Any) -> None:
    monkeypatch.setattr(
        "shared_storage.check_health.settings.BASE_STORAGE_URL", "http://localhost:1"
    )

    with (
        patch("src.service.nats_connect") as mock_nats_connect,
        pytest.raises(Exception),
    ):
        await start_service()

    mock_nats_connect.assert_not_called()
