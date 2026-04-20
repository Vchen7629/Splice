from shared_core.logging import logger
from shared_handler.kv import create_kv
from shared_handler.kv import connect_kv
from shared_handler.connection import nats_connect
from shared_handler.connection import check_js_stream_exists
from shared_handler.http_server import start_health_server
from shared_storage.check_health import check_storage_health
from .handler.subscriber import raw_videos
from .core.settings import settings
import asyncio


async def start_service() -> None:
    """Start the python scene-detection service"""
    check_storage_health()
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect()

    await check_js_stream_exists(js, settings.SCENE_SPLIT_SUBJECT)
    await check_js_stream_exists(js, settings.VIDEO_CHUNKS_SUBJECT)

    job_status_kv = await connect_kv(js, "job-status")
    msg_processed_kv = await create_kv(js, "scene-split-processed")

    try:
        await raw_videos(js, msg_processed_kv, job_status_kv)
    finally:
        health_server.shutdown()
        await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(start_service())
