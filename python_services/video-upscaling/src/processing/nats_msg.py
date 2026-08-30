from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.kv import KeyValue
from nats.js import JetStreamContext
from shared_core import get_logger, settings as shared_settings
from shared_handler import (
    publisher,
    keep_alive,
    update_job_stage,
    update_job_failed,
    check_already_processed,
    ProcessJobMessage,
    UpscaleCompleteMsg,
    ProgressReporter,
)
from shared_storage import fetch_video, upload_video
from ..core.settings import settings
from .video import video_upscale, video_downscale, recombine_video_audio
from utils import select_model
from pathlib import Path
import os
import shutil
import asyncio

logger = get_logger(settings.SERVICE_NAME)


async def process_msg(
    nc: NATSClient,
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    job_stage_kv: KeyValue,
    msg: Msg,
) -> None:
    """Processes a single video upscale nats message"""
    metadata: ProcessJobMessage | None = None
    try:
        metadata = ProcessJobMessage.model_validate_json(msg.data.decode())

        if await check_already_processed(msg_processed_kv, metadata.job_id):
            logger.debug("job already processed, skipping", job_id=metadata.job_id)
            await msg.ack()
            return

        await update_job_stage(
            job_stage_kv, metadata.job_id, settings.SERVICE_NAME, settings.SERVICE_NAME
        )

        async with keep_alive(
            settings.SERVICE_NAME, msg, interval=shared_settings.ACK_WAIT_S / 3
        ):
            local_video_path = await asyncio.to_thread(
                fetch_video, metadata.storage_url, settings.SERVICE_NAME
            )
            filename = os.path.basename(local_video_path)
            temp_file_loc = f"../temp_output/{metadata.job_id}/{filename}"
            os.makedirs(os.path.dirname(temp_file_loc), exist_ok=True)

            logger.debug(
                "fetched unprocessed video",
                job_id=metadata.job_id,
                saved_to=local_video_path,
            )

            res = select_model(metadata.source_resolution, metadata.target_resolution)
            if res is None:
                await _downscale_job(
                    nc,
                    js,
                    msg,
                    msg_processed_kv,
                    metadata,
                    local_video_path,
                    temp_file_loc,
                )
                return

            await _upscale_job(
                nc,
                js,
                msg_processed_kv,
                job_stage_kv,
                msg,
                metadata,
                local_video_path,
                res,
            )
            return
    except Exception as e:
        logger.error("unexpected error processing job", err=str(e))
        if metadata is not None:
            try:
                await update_job_failed(
                    job_stage_kv, metadata.job_id, str(e), settings.SERVICE_NAME
                )
            except Exception:
                await msg.nak()
                return
            finally:
                shutil.rmtree(f"../temp_output/{metadata.job_id}", ignore_errors=True)
                shutil.rmtree(f"../temp/{metadata.job_id}", ignore_errors=True)
                logger.debug("removed temp dirs", job_id=metadata.job_id)
        await msg.ack()


async def _finalize_job(
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    msg: Msg,
    job_id: str,
    temp_file_loc: str,
) -> None:
    """shared logic for uploading video file to storage, publish complete msg, updating KV and acking msg"""
    storage_url = f"{settings.BASE_STORAGE_URL}/{job_id}/output.mp4/processed"
    upload_video(storage_url, job_id, temp_file_loc, settings.SERVICE_NAME)

    await publisher(
        js,
        UpscaleCompleteMsg(job_id=job_id),
        settings.PUB_SUBJECT,
        settings.SERVICE_NAME,
    )

    await msg_processed_kv.put(job_id, b"done")
    await msg.ack()

    shutil.rmtree(os.path.dirname(temp_file_loc))
    shutil.rmtree(f"../temp/{job_id}", ignore_errors=True)
    logger.debug("removed temp dirs", job_id=job_id)


async def _upscale_job(
    nc: NATSClient,
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    job_stage_kv: KeyValue,
    msg: Msg,
    metadata: ProcessJobMessage,
    local_video_path: str,
    res: tuple[Path, int],
) -> None:
    """upscale video path logic"""
    logger.debug(
        "upscaling video",
        job_id=metadata.job_id,
        source_res=metadata.source_resolution,
        target_res=metadata.target_resolution,
    )

    model_path, resolution_scale = res
    logger.debug(
        "upscaling with model and resolution",
        jobid=metadata,
        scale=resolution_scale,
        model=model_path,
    )

    # video_upscale always encodes to h264/mp4 regardless of the source
    # container, so the output must be saved with an .mp4 extension
    # reusing the source filename's extension (e.g. .webm) produces a
    # container/codec mismatch when recombine_video_audio muxes with -c copy
    stem = os.path.splitext(os.path.basename(local_video_path))[0]
    temp_file_loc = f"../temp_output/{metadata.job_id}/{stem}.mp4"
    os.makedirs(os.path.dirname(temp_file_loc), exist_ok=True)

    loop = asyncio.get_event_loop()
    upscale_reporter = ProgressReporter(
        nc, metadata.job_id, loop, settings.SERVICE_NAME
    )

    await asyncio.to_thread(
        video_upscale,
        metadata.job_id,
        local_video_path,
        model_path,
        resolution_scale,
        upscale_reporter,
    )
    logger.debug("upscaled video", job_id=metadata.job_id)
    await upscale_reporter.flush()

    await update_job_stage(
        job_stage_kv, metadata.job_id, "video-recombiner", settings.SERVICE_NAME
    )
    recombine_reporter = ProgressReporter(nc, metadata.job_id, loop, "video-recombiner")
    await asyncio.to_thread(
        recombine_video_audio,
        metadata.job_id,
        local_video_path,
        temp_file_loc,
        metadata.target_resolution,
        recombine_reporter,
    )
    logger.debug("recombined video with audio", job_id=metadata.job_id)
    await recombine_reporter.flush()

    await _finalize_job(js, msg_processed_kv, msg, metadata.job_id, temp_file_loc)


async def _downscale_job(
    nc: NATSClient,
    js: JetStreamContext,
    msg: Msg,
    msg_processed_kv: KeyValue,
    metadata: ProcessJobMessage,
    local_video_path: str,
    temp_file_loc: str,
) -> None:
    """downscale video path logic"""
    logger.debug(
        "downscaling video",
        job_id=metadata.job_id,
        source_res=metadata.source_resolution,
        target_res=metadata.target_resolution,
    )

    loop = asyncio.get_event_loop()
    downscale_reporter = ProgressReporter(
        nc, metadata.job_id, loop, settings.SERVICE_NAME
    )

    await asyncio.to_thread(
        video_downscale,
        local_video_path,
        metadata.target_resolution,
        temp_file_loc,
        downscale_reporter,
    )
    logger.debug("downscaled video", job_id=metadata.job_id)
    await downscale_reporter.flush()

    await _finalize_job(js, msg_processed_kv, msg, metadata.job_id, temp_file_loc)
