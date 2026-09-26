import asyncio
import os
from dataclasses import dataclass
from pathlib import Path
from threading import Event

from shared_core import get_logger
from shared_handler import (
    ProcessJobMsgContext,
    UpscaleCompleteMsg,
    publisher,
    update_job_stage,
)
from shared_storage import fetch_video, upload_video
from shared_util import ProgressReporter, cleanup_temp_dir, cleanup_temp_file

from utils import select_model

from ..core.settings import settings
from .video import recombine_video_audio, video_downscale, video_upscale

logger = get_logger(settings.SERVICE_NAME)
SERVICE_NAME = settings.SERVICE_NAME


@dataclass
class UpscaleJobContext(ProcessJobMsgContext):
    local_video_path: str
    res: tuple[Path, int]


@dataclass
class DownscaleJobContext(ProcessJobMsgContext):
    local_video_path: str
    temp_file_loc: str


async def process_job_msg(ctx: ProcessJobMsgContext, cancel_event: Event) -> None:
    """TODO: docstring"""
    job_id = ctx.metadata.job_id

    local_video_path = await asyncio.to_thread(
        fetch_video, ctx.metadata.storage_url, SERVICE_NAME
    )

    logger.debug(
        "fetched unprocessed video",
        job_id=job_id,
        saved_to=local_video_path,
    )

    res = select_model(ctx.metadata.source_resolution, ctx.metadata.target_resolution)
    if res is None:
        filename = os.path.basename(local_video_path)
        temp_file_loc = f"../temp_output/{job_id}/{filename}"
        os.makedirs(os.path.dirname(temp_file_loc), exist_ok=True)

        downscaleJobCtx = DownscaleJobContext(
            ctx.nc,
            ctx.js,
            ctx.msg_processed_kv,
            ctx.job_milestone_kv,
            ctx.metadata,
            local_video_path,
            temp_file_loc,
        )

        await _downscale_job(downscaleJobCtx, cancel_event)
        return

    upscaleJobContext = UpscaleJobContext(
        ctx.nc,
        ctx.js,
        ctx.msg_processed_kv,
        ctx.job_milestone_kv,
        ctx.metadata,
        local_video_path,
        res,
    )

    await _upscale_job(upscaleJobContext, cancel_event)


async def cleanup_job(job_id: str) -> None:
    """TODO: docstring"""
    await cleanup_temp_dir(f"../temp_output/{job_id}", job_id, logger)
    await cleanup_temp_dir(f"../temp/{job_id}", job_id, logger)

    logger.debug("removed temp dirs", job_id=job_id)


async def _upscale_job(ctx: UpscaleJobContext, cancel_event: Event) -> None:
    """upscale video path logic"""
    job_id = ctx.metadata.job_id

    logger.debug(
        "upscaling video",
        job_id=job_id,
        source_res=ctx.metadata.source_resolution,
        target_res=ctx.metadata.target_resolution,
    )

    model_path, resolution_scale = ctx.res
    logger.debug(
        "upscaling with model and resolution",
        jobid=ctx.metadata.job_id,
        scale=resolution_scale,
        model=model_path,
    )

    # video_upscale always encodes to h264/mp4 regardless of the source
    # container, so the output must be saved with an .mp4 extension
    # reusing the source filename's extension (e.g. .webm) produces a
    # container/codec mismatch when recombine_video_audio muxes with -c copy
    stem = os.path.splitext(os.path.basename(ctx.local_video_path))[0]
    temp_file_loc = f"../temp_output/{job_id}/{stem}.mp4"
    os.makedirs(os.path.dirname(temp_file_loc), exist_ok=True)

    loop = asyncio.get_event_loop()
    upscale_reporter = ProgressReporter(ctx.nc, job_id, loop, SERVICE_NAME)

    try:
        await asyncio.to_thread(
            video_upscale,
            cancel_event,
            job_id,
            ctx.local_video_path,
            model_path,
            resolution_scale,
            upscale_reporter,
        )
        logger.debug("upscaled video", job_id=job_id)
        await upscale_reporter.flush()

        await update_job_stage(
            ctx.job_milestone_kv, job_id, "video-recombiner", SERVICE_NAME
        )
        recombine_reporter = ProgressReporter(ctx.nc, job_id, loop, "video-recombiner")
        await asyncio.to_thread(
            recombine_video_audio,
            job_id,
            ctx.local_video_path,
            temp_file_loc,
            ctx.metadata.target_resolution,
            recombine_reporter,
        )
        logger.debug("recombined video with audio", job_id=job_id)
        await recombine_reporter.flush()

    finally:
        logger.debug("cleaning up no audio upscale mp4 file", job_id=job_id)
        await cleanup_temp_file(f"/tmp/upscaled_noaudio-{job_id}.mp4", job_id, logger)

    storage_url = f"{settings.BASE_STORAGE_URL}/{job_id}/output.mp4/processed"
    upload_video(storage_url, job_id, temp_file_loc, SERVICE_NAME)

    await publisher(
        ctx.js,
        UpscaleCompleteMsg(job_id=job_id),
        settings.PUB_SUBJECT,
        SERVICE_NAME,
    )


async def _downscale_job(ctx: DownscaleJobContext, cancel_event: Event) -> None:
    """downscale video path logic"""
    job_id = ctx.metadata.job_id

    logger.debug(
        "downscaling video",
        job_id=job_id,
        source_res=ctx.metadata.source_resolution,
        target_res=ctx.metadata.target_resolution,
    )

    loop = asyncio.get_event_loop()
    downscale_reporter = ProgressReporter(ctx.nc, job_id, loop, SERVICE_NAME)

    await asyncio.to_thread(
        video_downscale,
        cancel_event,
        ctx.local_video_path,
        ctx.metadata.target_resolution,
        ctx.temp_file_loc,
        downscale_reporter,
    )
    logger.debug("downscaled video", job_id=job_id)
    await downscale_reporter.flush()

    storage_url = f"{settings.BASE_STORAGE_URL}/{job_id}/output.mp4/processed"
    upload_video(storage_url, job_id, ctx.temp_file_loc, SERVICE_NAME)

    await publisher(
        ctx.js,
        UpscaleCompleteMsg(job_id=job_id),
        settings.PUB_SUBJECT,
        SERVICE_NAME,
    )
