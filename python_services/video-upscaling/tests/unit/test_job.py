from pathlib import Path
from threading import Event
from typing import Any, Generator
from unittest.mock import ANY, AsyncMock, MagicMock, patch

import pytest

from core import output_dir, settings
from job import job_orchestrator
from msgbus import JobContext, ProcessJobMessage, ProgressReporter, UpscaleCompleteMsg

MODEL = Path("/weights/model.pth")


def make_job(**overrides: Any) -> ProcessJobMessage:
    fields: dict[str, Any] = dict(
        job_id="job-123",
        storage_url="http://storage/video.mp4",
        source_resolution="480p",
        target_resolution="1080p",
        operation="upscale",
    )
    fields.update(overrides)
    return ProcessJobMessage(**fields)


@pytest.fixture
def job_patches() -> Generator[dict[str, Any], Any, None]:
    """Patches all external dependencies used by job_orchestrator."""
    with (
        patch("job.update_job_stage", new_callable=AsyncMock) as mock_update_stage,
        patch("job.fetch_video", return_value="/tmp/job-123/video.mp4") as mock_fetch,
        patch("job.select_model") as mock_select,
        patch("job.video_upscale") as mock_upscale,
        patch("job.video_downscale") as mock_downscale,
        patch("job.recombine_video_audio") as mock_recombine,
        patch("job.upload_video") as mock_upload,
        patch("job.publisher", new_callable=AsyncMock) as mock_pub,
        patch("job.makedirs"),
        patch(
            "job.to_thread",
            side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs),  # pyrefly: ignore[implicit-any-lambda]
        ),
    ):
        yield {
            "update_stage": mock_update_stage,
            "fetch": mock_fetch,
            "select": mock_select,
            "upscale": mock_upscale,
            "downscale": mock_downscale,
            "recombine": mock_recombine,
            "upload": mock_upload,
            "pub": mock_pub,
        }


@pytest.mark.asyncio
async def test_upscale_path_calls_video_upscale(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = (MODEL, 2)

    await job_orchestrator(job_ctx, make_job(), Event(), mock_logger)

    job_patches["upscale"].assert_called_once()
    job_patches["downscale"].assert_not_called()


@pytest.mark.asyncio
async def test_downscale_path_calls_video_downscale(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = None
    job = make_job(source_resolution="1080p", target_resolution="480p")

    await job_orchestrator(job_ctx, job, Event(), mock_logger)

    job_patches["downscale"].assert_called_once()
    job_patches["upscale"].assert_not_called()


@pytest.mark.asyncio
async def test_upscale_passes_correct_args(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = (MODEL, 4)
    job_patches["fetch"].return_value = "/tmp/video.mp4"

    await job_orchestrator(job_ctx, make_job(job_id="abc"), Event(), mock_logger)

    job_patches["upscale"].assert_called_once_with(
        ANY, "abc", "/tmp/video.mp4", MODEL, 4, "1080p", ANY
    )


@pytest.mark.asyncio
async def test_downscale_passes_correct_args(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = None
    job_patches["fetch"].return_value = "/tmp/video.mp4"
    job = make_job(job_id="abc", source_resolution="1080p", target_resolution="480p")

    await job_orchestrator(job_ctx, job, Event(), mock_logger)

    job_patches["downscale"].assert_called_once_with(
        ANY, "/tmp/video.mp4", "480p", f"{output_dir('abc')}/video.mp4", ANY
    )


@pytest.mark.asyncio
async def test_video_downscale_raises(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = None
    job_patches["downscale"].side_effect = RuntimeError("ffmpeg failed")
    job = make_job(source_resolution="1080p", target_resolution="480p")

    with pytest.raises(RuntimeError, match="ffmpeg failed"):
        await job_orchestrator(job_ctx, job, Event(), mock_logger)


@pytest.mark.asyncio
async def test_recombiner_stage_transition_waits_for_progress_flush(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    """job_orchestrator must not advance to the video-recombiner stage until all
    queued progress updates from video_upscale have been flushed"""
    job_patches["select"].return_value = (MODEL, 2)
    call_order: list[str] = []

    async def fake_flush(self: ProgressReporter) -> None:
        call_order.append("flush")

    async def fake_update_stage(kv: Any, job_id: str, stage: str) -> None:
        call_order.append(f"update_stage:{stage}")

    job_patches["update_stage"].side_effect = fake_update_stage

    with patch("job.ProgressReporter.flush", new=fake_flush):
        await job_orchestrator(job_ctx, make_job(), Event(), mock_logger)

    job_patches["recombine"].assert_called_once()
    assert call_order == ["flush", "update_stage:video-recombiner", "flush"]


@pytest.mark.asyncio
async def test_uploads_to_correct_storage_url(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = (MODEL, 2)

    await job_orchestrator(job_ctx, make_job(job_id="job-abc"), Event(), mock_logger)

    expected_url = f"{settings.BASE_STORAGE_URL}/job-abc/output.mp4/processed"
    job_patches["upload"].assert_called_once_with(expected_url, "job-abc", ANY)


@pytest.mark.asyncio
async def test_publishes_upscale_complete_msg(
    job_patches: dict[str, Any], job_ctx: JobContext, mock_logger: MagicMock
) -> None:
    job_patches["select"].return_value = (MODEL, 2)

    await job_orchestrator(job_ctx, make_job(job_id="job-abc"), Event(), mock_logger)

    job_patches["pub"].assert_called_once_with(
        job_ctx.js, UpscaleCompleteMsg(job_id="job-abc")
    )
