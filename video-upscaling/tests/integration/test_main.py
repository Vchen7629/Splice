import asyncio
import json
from typing import Any
from unittest.mock import AsyncMock, patch

import pytest
from nats.js import JetStreamContext

from core import settings
from main import run_service
from storage.queries import upload_video


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
            "operation": "upscale",
        }
    ).encode()


async def _run_service_until_processed(
    nc: Any,
    payload: bytes,
    queue_name: str,
    done: asyncio.Event,
    monkeypatch: pytest.MonkeyPatch,
    timeout: float = 30.0,
) -> None:
    monkeypatch.setattr(settings, "SUB_QUEUE_NAME", queue_name)
    nc.drain = AsyncMock()
    task = asyncio.create_task(run_service())
    await nc.publish(settings.SUB_SUBJECT, payload)
    await asyncio.wait_for(done.wait(), timeout=timeout)
    await asyncio.sleep(0.1)  # let NATS callbacks fire
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass


@pytest.mark.asyncio
async def test_full_job_publishes_upscale_complete_msg(
    patched_start_service: tuple[Any, JetStreamContext],
    complete_subscriber: tuple[list[dict[str, Any]], asyncio.Event],
    seeded_video: tuple[str, str],
    seaweedfs_url: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Full flow: message published -> video fetched from SeaweedFS -> downscaled -> UpscaleCompleteMsg on downstream subject."""
    monkeypatch.setattr(settings, "BASE_STORAGE_URL", seaweedfs_url)
    nc, _ = patched_start_service
    received, done = complete_subscriber
    job_id, storage_url = seeded_video

    await _run_service_until_processed(
        nc,
        _make_payload(job_id, storage_url),
        "test-full-flow-worker",
        done,
        monkeypatch,
    )

    assert any(m.get("job_id") == job_id for m in received)


@pytest.mark.asyncio
async def test_full_job_uploads_output_to_storage(
    patched_start_service: tuple[Any, JetStreamContext],
    seeded_video: tuple[str, str],
    seaweedfs_url: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """After processing, output video is PUT to SeaweedFS at the expected URL."""
    monkeypatch.setattr(settings, "BASE_STORAGE_URL", seaweedfs_url)
    nc, _ = patched_start_service
    job_id, storage_url = seeded_video
    uploaded_urls: list[str] = []
    done = asyncio.Event()

    def spy_upload(url: str, *args: Any, **kwargs: Any) -> str:
        uploaded_urls.append(url)
        result = upload_video(url, *args, **kwargs)
        done.set()
        return result

    with patch("job.upload_video", side_effect=spy_upload):
        await _run_service_until_processed(
            nc,
            _make_payload(job_id, storage_url),
            "test-upload-worker",
            done,
            monkeypatch,
        )

    expected_url = f"{seaweedfs_url}/{job_id}/output.mp4/processed"
    assert any(expected_url in url for url in uploaded_urls)


# ---------------------------------------------------------------------------
# startup failures
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
@pytest.mark.parametrize("subject_setting", ["PUB_SUBJECT", "SUB_SUBJECT"])
async def test_raises_when_stream_not_found(
    patched_start_service: tuple[Any, JetStreamContext],
    subject_setting: str,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    nc, _ = patched_start_service
    monkeypatch.setattr(settings, subject_setting, "nonexistent.subject.xyz")
    nc.drain = AsyncMock()

    with pytest.raises(RuntimeError, match="No stream found"):
        await run_service()


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
        patch("main.consumer", side_effect=RuntimeError("boom")),
        pytest.raises(RuntimeError),
    ):
        await run_service()

    assert called


@pytest.mark.asyncio
async def test_drain_called_on_cancellation(
    patched_start_service: tuple[Any, JetStreamContext],
    spy_drain: tuple[Any, list[bool]],
) -> None:
    _, called = spy_drain

    async def _hang(*_: Any, **__: Any) -> None:
        await asyncio.Event().wait()

    with patch("main.consumer", side_effect=_hang):
        task = asyncio.create_task(run_service())
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
async def test_raises_before_nats_when_storage_unreachable(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(settings, "BASE_STORAGE_URL", "http://localhost:1")

    with (
        patch("main.nats_connect") as mock_nats_connect,
        pytest.raises(Exception),
    ):
        await run_service()

    mock_nats_connect.assert_not_called()
