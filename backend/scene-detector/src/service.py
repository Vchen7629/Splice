from shared_handler.nats import consumer
from shared_core.logging import logger
from shared_handler.kv import create_kv
from shared_handler.kv import connect_kv
from shared_handler.connection import nats_connect
from shared_handler.connection import check_js_stream_exists
from shared_handler.http import start_health_server
from shared_storage.check_health import check_storage_health
from .core.settings import settings
from .processing.nats_msg import process_msg
import asyncio


async def start_service() -> None:
    """Start the python scene-detection service"""
    check_storage_health()
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect()

    await check_js_stream_exists(js, settings.SUB_SUBJECT)
    await check_js_stream_exists(js, settings.PUB_SUBJECT)

    job_status_kv = await connect_kv(js, "job-status")
    msg_processed_kv = await create_kv(js, "scene-split-processed")

    try:
        await consumer(
            js,
            msg_processed_kv,
            job_status_kv,
            settings.SUB_SUBJECT,
            settings.SUB_QUEUE_NAME,
            settings.SUB_QUEUE_NAME,
            process_msg=process_msg,
        )

    finally:
        health_server.shutdown()
        await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(start_service())
