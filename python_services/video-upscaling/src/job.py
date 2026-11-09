from asyncio import to_thread
from os import makedirs, path
from pathlib import Path
from threading import Event

from structlog.stdlib import BoundLogger

from core import output_dir, settings
from msgbus import (
    JobContext,
    ProcessJobMessage,
    ProgressReporter,
    UpscaleCompleteMsg,
    publisher,
    update_job_stage,
)
from processing import (
    recombine_video_audio,
    select_model,
    video_downscale,
    video_upscale,
)
from storage import fetch_video, upload_video


async def job_orchestrator(
    ctx: JobContext,
    job: ProcessJobMessage,
    cancel_event: Event,
    logger: BoundLogger,
) -> None:
    """Fetch the source video, upscale/downscale it to target resolution, upload result and pub completion"""
    src_path = await to_thread(fetch_video, job.storage_url)
    logger.debug("fetched unprocessed video", job_id=job.job_id, saved_to=src_path)

    makedirs(output_dir(job.job_id), exist_ok=True)

    model = select_model(job.source_resolution, target_res=job.target_resolution)
    if model is None:
        output_path = await _downscale_job(ctx, job, src_path, cancel_event, logger)
    else:
        output_path = await _upscale(ctx, job, src_path, model, cancel_event, logger)

    storage_url = f"{settings.BASE_STORAGE_URL}/{job.job_id}/output.mp4/processed"
    await to_thread(upload_video, storage_url, job.job_id, output_path)
    await publisher(ctx.js, UpscaleCompleteMsg(job_id=job.job_id))


async def _upscale(
    ctx: JobContext,
    job: ProcessJobMessage,
    src_path: str,
    model: tuple[Path, int],
    cancel_event: Event,
    logger: BoundLogger,
) -> str:
    """upscale video path logic and returns storage_url of the upscaled video"""
    model_path, scale = model
    logger.debug("upscaling video", job_id=job.job_id, scale=scale, model=model_path)

    # video_upscale always encodes h264, so the output must be .mp4 regardless of the
    # source container, or recombine_video_audio's -c copy mux hits a codec mismatch
    stem = path.splitext(path.basename(src_path))[0]
    output_path = f"{output_dir(job.job_id)}/{stem}.mp4"

    upscale_reporter = ProgressReporter(ctx.nc, job.job_id)
    await to_thread(
        video_upscale,
        cancel_event,
        job.job_id,
        src_path,
        model_path,
        scale,
        job.target_resolution,
        upscale_reporter,
    )
    await upscale_reporter.flush()

    await update_job_stage(ctx.milestone_kv, job.job_id, "video-recombiner")
    recombine_reporter = ProgressReporter(ctx.nc, job.job_id, "video-recombiner")
    await to_thread(
        recombine_video_audio,
        job.job_id,
        src_path,
        output_path,
        recombine_reporter,
    )
    await recombine_reporter.flush()

    return output_path


async def _downscale_job(
    ctx: JobContext,
    job: ProcessJobMessage,
    src_path: str,
    cancel_event: Event,
    logger: BoundLogger,
) -> str:
    """downscale video path logic and returns the storage_url of the downscaled video"""
    logger.debug(
        "downscaling video",
        job_id=job.job_id,
        target_res=job.target_resolution,
    )
    output_path = f"{output_dir(job.job_id)}/{path.basename(src_path)}"

    reporter = ProgressReporter(ctx.nc, job.job_id)
    await to_thread(
        video_downscale,
        cancel_event,
        src_path,
        job.target_resolution,
        output_path,
        reporter,
    )
    await reporter.flush()

    return output_path
