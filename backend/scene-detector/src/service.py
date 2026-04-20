from shared_handler.kv import create_msg_processed_kv
from shared_core.logging import logger
from shared_handler.connection import nats_connect
from shared_handler.http_server import start_health_server
from shared_storage.check_health import check_storage_health
from .handler.subscriber import raw_videos
from .core.settings import settings
import nats.js.errors as js_errors
import asyncio


async def start_service() -> None:
    """Start the python scene-detection service"""
    check_storage_health()
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect()

    try:
        await js.find_stream_name_by_subject(settings.SCENE_SPLIT_SUBJECT)
    except js_errors.NotFoundError:
        raise RuntimeError(
            f"No stream found for subscriber `{settings.SCENE_SPLIT_SUBJECT}`"
        )

    try:
        await js.find_stream_name_by_subject(settings.VIDEO_CHUNKS_SUBJECT)
    except js_errors.NotFoundError:
        raise RuntimeError(
            f"No stream found for video chunks subject `{settings.VIDEO_CHUNKS_SUBJECT}`"
        )

    try:
        job_status_kv = await js.key_value("job-status")
    except js_errors.NotFoundError:
        raise RuntimeError(
            "job-status KV bucket not found, check video-status is running"
        )

    msg_processed_kv = await create_msg_processed_kv("scene-split-processed", js)

    try:
        await raw_videos(js, msg_processed_kv, job_status_kv)
    finally:
        health_server.shutdown()
        await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(start_service())
