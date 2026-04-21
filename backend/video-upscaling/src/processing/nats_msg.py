import shutil
from nats.aio.msg import Msg
from nats.js.kv import KeyValue
from nats.js import JetStreamContext
from shared_core.logging import get_logger
from shared_handler.nats import publisher
from shared_handler.kv import update_job_status
from shared_handler.kv import check_already_processed
from shared_handler.messages import ProcessJobMessage
from shared_handler.messages import UpscaleCompleteMsg
from shared_storage.queries import fetch_video
from shared_storage.queries import upload_video
from core.settings import settings
from processing.video import video_upscale
from processing.video import video_downscale
from src.utils.model_router import select_model
import os
import asyncio

logger = get_logger(settings.SERVICE_NAME)

async def process_msg(
    js: JetStreamContext, msg_processed_kv: KeyValue, job_status_kv: KeyValue, msg: Msg
) -> None:
    """Processes a single video upscale nats message"""
    try:
        metadata = ProcessJobMessage.model_validate_json(msg.data.decode())

        if await check_already_processed(msg_processed_kv, metadata.job_id):
            logger.debug("job already processed, skipping", job_id=metadata.job_id)
            await msg.ack()
            return

        await update_job_status(job_status_kv, metadata.job_id, settings.SERVICE_NAME, settings.SERVICE_NAME)

        local_video_path = await asyncio.to_thread(fetch_video, metadata.storage_url, settings.SERVICE_NAME)
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
            logger.debug(
                "downscaling video",
                job_id=metadata.job_id,
                source_res=metadata.source_resolution,
                target_res=metadata.target_resolution,
            )

            # async since its very light ffmpeg subprocess
            await asyncio.to_thread(
                video_downscale,
                local_video_path,
                metadata.target_resolution,
                temp_file_loc,
            )
            logger.debug("downscaled video", job_id=metadata.job_id)

            await _finalize_job(
                js, msg_processed_kv, msg, metadata.job_id, temp_file_loc
            )

            return

        logger.debug(
            "upscaling video",
            job_id=metadata.job_id,
            source_res=metadata.source_resolution,
            target_res=metadata.target_resolution,
        )

        model_path, resolution_scale = res
        logger.debug("upscaling with model and resolution", jobid=metadata, scale=resolution_scale, model=model_path)

        video_upscale(local_video_path, temp_file_loc, model_path, resolution_scale)
        logger.debug("upscaled video", job_id=metadata.job_id)

        await _finalize_job(js, msg_processed_kv, msg, metadata.job_id, temp_file_loc)

        return
    except Exception as e:
        logger.error("unexpected error processing job", err=str(e))
        await msg.nak()


async def _finalize_job(
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    msg: Msg,
    job_id: str,
    temp_file_loc: str,
) -> None:
    """shared logic for uploading video file to storage, publish complete msg, updating KV and acking msg"""
    upload_video(job_id, temp_file_loc, settings.SERVICE_NAME)

    await publisher(js, UpscaleCompleteMsg(job_id=job_id), settings.PUB_SUBJECT, settings.SERVICE_NAME)
    
    await msg_processed_kv.put(job_id, b"done")
    await msg.ack()
    
    temp_dir = os.path.dirname(temp_file_loc)
    shutil.rmtree(temp_dir)
    logger.debug("removed temp dir", job_id=job_id, temp_dir=temp_dir)
