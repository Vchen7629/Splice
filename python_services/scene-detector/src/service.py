from shared_handler import (
    consumer,
    create_kv,
    connect_kv,
    nats_connect,
    check_js_stream_exists,
    start_health_server,
)
from shared_core.logging import get_logger
from shared_storage.check_health import check_storage_health
from .core.settings import settings
from .processing.nats_msg import process_msg
import asyncio

logger = get_logger(settings.SERVICE_NAME)


async def start_service() -> None:
    """Start the python scene-detection service"""
    check_storage_health(settings.SERVICE_NAME)
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect(settings.SERVICE_NAME)

    try:
        await check_js_stream_exists(js, settings.SUB_SUBJECT)
        await check_js_stream_exists(js, settings.PUB_SUBJECT)

        job_milestone_kv = await connect_kv(js, "job-milestones")
        msg_processed_kv = await create_kv(js, "scene-split-processed")

        await consumer(
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
