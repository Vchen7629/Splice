from typing import Any
from pathlib import Path
from unittest.mock import ANY, AsyncMock, patch
from nats.aio.client import Client as NATSClient
from nats.js.kv import KeyValue
from nats.js.client import JetStreamContext
from src.core.settings import settings
from src.processing.nats_msg import process_msg, _finalize_job
from shared_handler import UpscaleCompleteMsg
from shared_util import ProgressReporter
from test_helpers.nats import make_msg
import pytest


MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_JS = AsyncMock(spec=JetStreamContext)
MOCK_KV = AsyncMock(spec=KeyValue)


@pytest.mark.asyncio
async def test_already_processed_acks_and_returns(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["check"].return_value = True
    msg = make_msg()

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

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

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["update_stage"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_path_calls_video_upscale(
    nats_msg_patches: dict[str, Any],
) -> None:
    model_path = Path("/weights/model.pth")
    nats_msg_patches["select"].return_value = (model_path, 2)
    msg = make_msg(source_resolution="480p", target_resolution="1080p")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["upscale"].assert_called_once()
    nats_msg_patches["downscale"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_removes_noaudio_temp_file(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    msg = make_msg(job_id="abc", source_resolution="480p", target_resolution="1080p")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["cleanup_temp_file"].assert_called_once_with(
        "/tmp/upscaled_noaudio-abc.mp4", "abc", ANY
    )


@pytest.mark.asyncio
async def test_downscale_path_calls_video_downscale(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = None
    msg = make_msg(source_resolution="1080p", target_resolution="480p")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["downscale"].assert_called_once()
    nats_msg_patches["upscale"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_passes_correct_args(nats_msg_patches: dict[str, Any]) -> None:
    model_path = Path("/weights/model.pth")
    nats_msg_patches["select"].return_value = (model_path, 4)
    nats_msg_patches["fetch"].return_value = "/tmp/video.mp4"
    msg = make_msg(job_id="abc", source_resolution="480p", target_resolution="1080p")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["upscale"].assert_called_once_with(
        "abc",
        "/tmp/video.mp4",
        model_path,
        4,
        ANY,
    )


@pytest.mark.asyncio
async def test_downscale_passes_correct_args(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = None
    nats_msg_patches["fetch"].return_value = "/tmp/video.mp4"
    msg = make_msg(job_id="abc", source_resolution="1080p", target_resolution="480p")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["downscale"].assert_called_once_with(
        "/tmp/video.mp4",
        "480p",
        "../temp_output/abc/video.mp4",
        ANY,
    )


@pytest.mark.asyncio
async def test_invalid_json_acks_without_updating_kv(
    nats_msg_patches: dict[str, Any],
) -> None:
    msg = AsyncMock()
    msg.data = b"not valid json"

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    nats_msg_patches["update_failed"].assert_not_called()


@pytest.mark.asyncio
async def test_fetch_video_raises_updates_kv_and_acks(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["fetch"].side_effect = RuntimeError("storage down")
    msg = make_msg()

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["update_failed"].assert_called_once_with(
        ANY, "job-123", "storage down", settings.SERVICE_NAME
    )
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_video_upscale_raises_updates_kv_and_acks(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    nats_msg_patches["upscale"].side_effect = RuntimeError("gpu oom")
    msg = make_msg()

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["update_failed"].assert_called_once_with(
        ANY, "job-123", "gpu oom", settings.SERVICE_NAME
    )
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


FLUSH_FAILURE_SIDE_EFFECTS = {
    "upscale_flush": [RuntimeError("boom"), None],
    "recombine_flush": [None, RuntimeError("boom")],
}


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "failure_point",
    [
        "video_upscale",
        "upscale_flush",
        "update_job_stage",
        "recombine_video_audio",
        "recombine_flush",
    ],
)
async def test_upscale_failure_still_cleans_up_noaudio_file(
    failure_point: str, nats_msg_patches: dict[str, Any]
) -> None:
    """cleanup_temp_file must run no matter which step of the upscale/recombine
    sequence fails, since a partial noaudio file may already be on disk"""
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)

    if failure_point == "video_upscale":
        nats_msg_patches["upscale"].side_effect = RuntimeError("boom")
    elif failure_point == "update_job_stage":
        nats_msg_patches["update_stage"].side_effect = [None, RuntimeError("boom")]
    elif failure_point == "recombine_video_audio":
        nats_msg_patches["recombine"].side_effect = RuntimeError("boom")

    msg = make_msg(job_id="abc")

    with patch(
        "src.processing.nats_msg.ProgressReporter.flush",
        new_callable=AsyncMock,
        side_effect=FLUSH_FAILURE_SIDE_EFFECTS.get(failure_point, [None, None]),
    ):
        await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["cleanup_temp_file"].assert_called_once_with(
        "/tmp/upscaled_noaudio-abc.mp4", "abc", ANY
    )
    nats_msg_patches["update_failed"].assert_called_once()
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_video_downscale_raises_updates_kv_and_acks(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = None
    nats_msg_patches["downscale"].side_effect = RuntimeError("ffmpeg failed")
    msg = make_msg(source_resolution="1080p", target_resolution="480p")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["update_failed"].assert_called_once_with(
        ANY, "job-123", "ffmpeg failed", settings.SERVICE_NAME
    )
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_finalize_uploads_to_correct_storage_url(
    nats_msg_patches: dict[str, Any],
) -> None:
    await _finalize_job(
        MOCK_JS, MOCK_KV, AsyncMock(), "job-abc", "/tmp/job-abc/output.mp4"
    )

    expected_url = f"{settings.BASE_STORAGE_URL}/job-abc/output.mp4/processed"
    nats_msg_patches["upload"].assert_called_once_with(
        expected_url, "job-abc", "/tmp/job-abc/output.mp4", settings.SERVICE_NAME
    )


@pytest.mark.asyncio
async def test_finalize_publishes_upscale_complete_msg(
    nats_msg_patches: dict[str, Any],
) -> None:
    await _finalize_job(MOCK_JS, MOCK_KV, AsyncMock(), "job-abc", "/tmp/out.mp4")

    nats_msg_patches["pub"].assert_called_once_with(
        MOCK_JS,
        UpscaleCompleteMsg(job_id="job-abc"),
        settings.PUB_SUBJECT,
        settings.SERVICE_NAME,
    )


@pytest.mark.asyncio
async def test_finalize_marks_job_processed_in_kv(
    nats_msg_patches: dict[str, Any],
) -> None:
    mock_kv = AsyncMock()
    await _finalize_job(MOCK_JS, mock_kv, AsyncMock(), "job-abc", "/tmp/out.mp4")

    mock_kv.put.assert_called_once_with("job-abc", b"done")


@pytest.mark.asyncio
async def test_finalize_acks_message(nats_msg_patches: dict[str, Any]) -> None:
    msg = AsyncMock()
    await _finalize_job(MOCK_JS, MOCK_KV, msg, "job-abc", "/tmp/out.mp4")

    msg.ack.assert_called_once()


@pytest.mark.asyncio
async def test_process_msg_removes_temp_dirs_on_success(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    msg = make_msg(job_id="job-abc")

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    cleanup_calls = nats_msg_patches["cleanup_temp_dir"].call_args_list
    removed_paths = [str(c.args[0]) for c in cleanup_calls]
    assert any("job-abc" in p for p in removed_paths)


@pytest.mark.asyncio
async def test_downscale_path_removes_temp_dirs_on_success(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = None
    nats_msg_patches["fetch"].return_value = "/tmp/video.mp4"
    msg = make_msg(
        job_id="job-abc", source_resolution="1080p", target_resolution="480p"
    )

    await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["downscale"].assert_called_once_with(
        "/tmp/video.mp4",
        "480p",
        "../temp_output/job-abc/video.mp4",
        ANY,
    )

    cleanup_calls = nats_msg_patches["cleanup_temp_dir"].call_args_list
    removed_paths = [str(c.args[0]) for c in cleanup_calls]
    assert any("job-abc" in p for p in removed_paths)


@pytest.mark.asyncio
async def test_recombiner_stage_transition_waits_for_progress_flush(
    nats_msg_patches: dict[str, Any],
) -> None:
    """process_msg must not advance to the video-recombiner stage until all
    queued progress updates from video_upscale have been flushed"""
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    call_order: list[str] = []

    async def fake_flush(self: ProgressReporter) -> None:
        call_order.append("flush")

    async def fake_update_stage(kv: Any, job_id: str, stage: str, service: str) -> None:
        if stage == "video-recombiner":
            call_order.append("update_stage:video-recombiner")

    nats_msg_patches["update_stage"].side_effect = fake_update_stage
    msg = make_msg()

    with patch("src.processing.nats_msg.ProgressReporter.flush", new=fake_flush):
        await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    nats_msg_patches["recombine"].assert_called_once()
    assert call_order == ["flush", "update_stage:video-recombiner", "flush"]
