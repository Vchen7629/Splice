import asyncio

from shared_core import get_logger
from shared_handler import (
    check_js_stream_exists,
    connect_kv,
    consumer,
    create_kv,
    nats_connect,
    start_health_server,
)
from shared_storage import check_storage_health

from .core.settings import settings
from .processing.nats_msg import process_msg

logger = get_logger(settings.SERVICE_NAME)


async def start_service() -> None:
    """Start the video-upscaling service"""
    check_storage_health(settings.SERVICE_NAME)
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect(settings.SERVICE_NAME)

    try:
        await check_js_stream_exists(js, settings.SUB_SUBJECT)
        await check_js_stream_exists(js, settings.PUB_SUBJECT)

        job_milestone_kv = await connect_kv(js, "job-milestones")
        msg_processed_kv = await create_kv(js, "upscale-processed")

        await consumer(
            logger,
            nc,
            js,
            msg_processed_kv,
            job_milestone_kv,
            settings.SUB_SUBJECT,
            settings.SUB_QUEUE_NAME,
            settings.SUB_QUEUE_NAME,
            process_msg=process_msg,
        )
    finally:
        health_server.shutdown()
        if not nc.is_closed:
            await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(start_service())
