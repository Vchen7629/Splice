from pathlib import Path
from typing import Any
from unittest.mock import ANY, AsyncMock, patch

import pytest
from nats.aio.client import Client as NATSClient
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from shared_handler import ProcessJobMessage, ProcessJobMsgContext, UpscaleCompleteMsg
from shared_util import ProgressReporter

from src.core.settings import settings
from src.processing.nats_msg import cleanup_job, process_job_msg

MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_JS = AsyncMock(spec=JetStreamContext)
MOCK_KV = AsyncMock(spec=KeyValue)


def make_ctx(**overrides: Any) -> ProcessJobMsgContext:
    metadata = overrides.pop(
        "metadata",
        ProcessJobMessage(
            job_id="job-123",
            storage_url="http://storage/video.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        ),
    )
    defaults: dict[str, Any] = dict(
        nc=MOCK_NC, js=MOCK_JS, msg_processed_kv=MOCK_KV, job_milestone_kv=MOCK_KV
    )
    defaults.update(overrides)
    return ProcessJobMsgContext(metadata=metadata, **defaults)


@pytest.mark.asyncio
async def test_upscale_path_calls_video_upscale(
    nats_msg_patches: dict[str, Any],
) -> None:
    model_path = Path("/weights/model.pth")
    nats_msg_patches["select"].return_value = (model_path, 2)
    ctx = make_ctx()

    await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["upscale"].assert_called_once()
    nats_msg_patches["downscale"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_removes_noaudio_temp_file(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="abc",
            storage_url="http://storage/video.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        )
    )

    await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["cleanup_temp_file"].assert_called_once_with(
        "/tmp/upscaled_noaudio-abc.mp4", "abc", ANY
    )


@pytest.mark.asyncio
async def test_downscale_path_calls_video_downscale(
    nats_msg_patches: dict[str, Any],
) -> None:
    nats_msg_patches["select"].return_value = None
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="job-123",
            storage_url="http://storage/video.mp4",
            source_resolution="1080p",
            target_resolution="480p",
        )
    )

    await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["downscale"].assert_called_once()
    nats_msg_patches["upscale"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_passes_correct_args(nats_msg_patches: dict[str, Any]) -> None:
    model_path = Path("/weights/model.pth")
    nats_msg_patches["select"].return_value = (model_path, 4)
    nats_msg_patches["fetch"].return_value = "/tmp/video.mp4"
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="abc",
            storage_url="http://storage/video.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        )
    )

    await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["upscale"].assert_called_once_with(
        ANY,
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
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="abc",
            storage_url="http://storage/video.mp4",
            source_resolution="1080p",
            target_resolution="480p",
        )
    )

    await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["downscale"].assert_called_once_with(
        ANY,
        "/tmp/video.mp4",
        "480p",
        "../temp_output/abc/video.mp4",
        ANY,
    )


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
        nats_msg_patches["update_stage"].side_effect = RuntimeError("boom")
    elif failure_point == "recombine_video_audio":
        nats_msg_patches["recombine"].side_effect = RuntimeError("boom")

    flush_side_effect = {
        "upscale_flush": RuntimeError("boom"),
        "recombine_flush": [None, RuntimeError("boom")],
    }.get(failure_point, None)

    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="abc",
            storage_url="http://storage/video.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        )
    )

    with patch(
        "src.processing.nats_msg.ProgressReporter.flush",
        new_callable=AsyncMock,
        side_effect=flush_side_effect,
    ):
        with pytest.raises(RuntimeError):
            await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["cleanup_temp_file"].assert_called_once_with(
        "/tmp/upscaled_noaudio-abc.mp4", "abc", ANY
    )


@pytest.mark.asyncio
async def test_video_downscale_raises(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = None
    nats_msg_patches["downscale"].side_effect = RuntimeError("ffmpeg failed")
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="job-123",
            storage_url="http://storage/video.mp4",
            source_resolution="1080p",
            target_resolution="480p",
        )
    )

    with pytest.raises(RuntimeError, match="ffmpeg failed"):
        await process_job_msg(ctx, AsyncMock())


@pytest.mark.asyncio
async def test_recombiner_stage_transition_waits_for_progress_flush(
    nats_msg_patches: dict[str, Any],
) -> None:
    """process_job_msg must not advance to the video-recombiner stage until all
    queued progress updates from video_upscale have been flushed"""
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    call_order: list[str] = []

    async def fake_flush(self: ProgressReporter) -> None:
        call_order.append("flush")

    async def fake_update_stage(kv: Any, job_id: str, stage: str, service: str) -> None:
        if stage == "video-recombiner":
            call_order.append("update_stage:video-recombiner")

    nats_msg_patches["update_stage"].side_effect = fake_update_stage
    ctx = make_ctx()

    with patch("src.processing.nats_msg.ProgressReporter.flush", new=fake_flush):
        await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["recombine"].assert_called_once()
    assert call_order == ["flush", "update_stage:video-recombiner", "flush"]


@pytest.mark.asyncio
async def test_uploads_to_correct_storage_url(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="job-abc",
            storage_url="http://storage/video.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        )
    )

    await process_job_msg(ctx, AsyncMock())

    expected_url = f"{settings.BASE_STORAGE_URL}/job-abc/output.mp4/processed"
    nats_msg_patches["upload"].assert_called_once_with(
        expected_url, "job-abc", ANY, settings.SERVICE_NAME
    )


@pytest.mark.asyncio
async def test_publishes_upscale_complete_msg(nats_msg_patches: dict[str, Any]) -> None:
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    ctx = make_ctx(
        metadata=ProcessJobMessage(
            job_id="job-abc",
            storage_url="http://storage/video.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        )
    )

    await process_job_msg(ctx, AsyncMock())

    nats_msg_patches["pub"].assert_called_once_with(
        MOCK_JS,
        UpscaleCompleteMsg(job_id="job-abc"),
        settings.PUB_SUBJECT,
        settings.SERVICE_NAME,
    )


@pytest.mark.asyncio
async def test_cleanup_job_removes_temp_dirs(nats_msg_patches: dict[str, Any]) -> None:
    await cleanup_job("job-abc")

    cleanup_calls = nats_msg_patches["cleanup_temp_dir"].call_args_list
    removed_paths = [str(c.args[0]) for c in cleanup_calls]
    assert any("job-abc" in p for p in removed_paths)
