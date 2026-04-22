from typing import Any
from pathlib import Path
from unittest.mock import ANY
from unittest.mock import AsyncMock
from src.core.settings import settings
from src.processing.nats_msg import process_msg
from src.processing.nats_msg import _finalize_job
from shared_handler.messages import ProcessJobMessage
from shared_handler.messages import UpscaleCompleteMsg
import pytest


def make_msg(
    job_id: str = "job-123",
    storage_url: str = "http://storage/video.mp4",
    source_resolution: str = "480p",
    target_resolution: str = "1080p",
) -> AsyncMock:
    """Build a mock NATS Msg with a valid ProcessJobMessage payload."""
    payload = ProcessJobMessage(
        job_id=job_id,
        storage_url=storage_url,
        source_resolution=source_resolution,
        target_resolution=target_resolution,
    )
    msg = AsyncMock()
    msg.data = payload.model_dump_json().encode()
    return msg


@pytest.mark.asyncio
async def test_already_processed_acks_and_returns(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["check"].return_value = True
    msg = make_msg()

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    msg.ack.assert_called_once()
    nats_msg_patches["upscale"].assert_not_called()
    nats_msg_patches["downscale"].assert_not_called()
    nats_msg_patches["upload"].assert_not_called()


@pytest.mark.asyncio
async def test_already_processed_skips_status_update(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["check"].return_value = True
    msg = make_msg()

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    nats_msg_patches["update_status"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_path_calls_video_upscale(
    nats_msg_patches: dict[str, Any],
) -> None:
    model_path = Path("/weights/model.pth")
    nats_msg_patches["select"].return_value = (model_path, 2)
    msg = make_msg(source_resolution="480p", target_resolution="1080p")

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    nats_msg_patches["upscale"].assert_called_once()
    nats_msg_patches["downscale"].assert_not_called()


@pytest.mark.asyncio
async def test_downscale_path_calls_video_downscale(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = None
    msg = make_msg(source_resolution="1080p", target_resolution="480p")

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    nats_msg_patches["downscale"].assert_called_once()
    nats_msg_patches["upscale"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_passes_correct_args(nats_msg_patches: dict[str, Any]) -> None:
    model_path = Path("/weights/model.pth")
    nats_msg_patches["select"].return_value = (model_path, 4)
    nats_msg_patches["fetch"].return_value = "/tmp/video.mp4"
    msg = make_msg(job_id="abc", source_resolution="480p", target_resolution="1080p")

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    nats_msg_patches["upscale"].assert_called_once_with(
        "/tmp/video.mp4",
        "../temp_output/abc/video.mp4",
        model_path,
        4,
        ANY,
    )


@pytest.mark.asyncio
async def test_downscale_passes_correct_args(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = None
    nats_msg_patches["fetch"].return_value = "/tmp/video.mp4"
    msg = make_msg(job_id="abc", source_resolution="1080p", target_resolution="480p")

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    nats_msg_patches["downscale"].assert_called_once_with(
        "/tmp/video.mp4",
        "480p",
        "../temp_output/abc/video.mp4",
    )


@pytest.mark.asyncio
async def test_invalid_json_naks(nats_msg_patches: dict[str, Any]) -> None:
    msg = AsyncMock()
    msg.data = b"not valid json"

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_fetch_video_raises_naks(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["fetch"].side_effect = RuntimeError("storage down")
    msg = make_msg()

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_video_upscale_raises_naks(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    nats_msg_patches["upscale"].side_effect = RuntimeError("gpu oom")
    msg = make_msg()

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_video_downscale_raises_naks(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = None
    nats_msg_patches["downscale"].side_effect = RuntimeError("ffmpeg failed")
    msg = make_msg(source_resolution="1080p", target_resolution="480p")

    await process_msg(AsyncMock(), AsyncMock(), AsyncMock(), msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_finalize_uploads_to_correct_storage_url(
    nats_msg_patches: dict[str, Any],
) -> None:
    await _finalize_job(
        AsyncMock(), AsyncMock(), AsyncMock(), "job-abc", "/tmp/job-abc/output.mp4"
    )

    expected_url = f"{settings.BASE_STORAGE_URL}/job-abc/output.mp4/processed"
    nats_msg_patches["upload"].assert_called_once_with(
        expected_url, "job-abc", "/tmp/job-abc/output.mp4", settings.SERVICE_NAME
    )


@pytest.mark.asyncio
async def test_finalize_publishes_upscale_complete_msg(
    nats_msg_patches: dict[str, Any],
) -> None:
    mock_js = AsyncMock()
    await _finalize_job(mock_js, AsyncMock(), AsyncMock(), "job-abc", "/tmp/out.mp4")

    nats_msg_patches["pub"].assert_called_once_with(
        mock_js,
        UpscaleCompleteMsg(job_id="job-abc"),
        settings.PUB_SUBJECT,
        settings.SERVICE_NAME,
    )


@pytest.mark.asyncio
async def test_finalize_marks_job_processed_in_kv(
    nats_msg_patches: dict[str, Any],
) -> None:
    mock_kv = AsyncMock()
    await _finalize_job(AsyncMock(), mock_kv, AsyncMock(), "job-abc", "/tmp/out.mp4")

    mock_kv.put.assert_called_once_with("job-abc", b"done")


@pytest.mark.asyncio
async def test_finalize_acks_message(nats_msg_patches: dict[str, Any]) -> None:
    msg = AsyncMock()
    await _finalize_job(AsyncMock(), AsyncMock(), msg, "job-abc", "/tmp/out.mp4")

    msg.ack.assert_called_once()


@pytest.mark.asyncio
async def test_finalize_removes_temp_dirs(nats_msg_patches: dict[str, Any]) -> None:
    await _finalize_job(
        AsyncMock(),
        AsyncMock(),
        AsyncMock(),
        "job-abc",
        "../temp_output/job-abc/video.mp4",
    )

    rmtree_calls = nats_msg_patches["rmtree"].call_args_list
    removed_paths = [str(c.args[0]) for c in rmtree_calls]
    assert any("job-abc" in p for p in removed_paths)
