import asyncio

from nats.js.api import ConsumerConfig

from core import get_logger, settings
from health import start_health_server
from job import job_orchestrator
from msgbus import (
    JobContext,
    check_js_stream_exists,
    consumer,
    create_kv,
    kv_connect,
    nats_connect,
)
from storage import check_storage_health

logger = get_logger(settings.SERVICE_NAME)


async def run_service() -> None:
    """Handles starting the video upscaling service
    It starts the health server, connects to nats jetstream, connects to job-milestones kv and creates
    the processed kv and starts the consumer poll loop. ALso handles graceful shutdown
    """
    check_storage_health(logger)
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect(logger)

    try:
        await check_js_stream_exists(js, settings.SUB_SUBJECT)
        await check_js_stream_exists(js, settings.PUB_SUBJECT)

        job_milestone_kv = await kv_connect(js, "job-milestones")
        msg_processed_kv = await create_kv(js, "upscale-processed")

        ctx = JobContext(nc, js, job_milestone_kv)
        sub = await js.subscribe(
            subject=settings.SUB_SUBJECT,
            durable=settings.SUB_QUEUE_NAME,
            queue=settings.SUB_QUEUE_NAME,
            config=ConsumerConfig(
                max_deliver=settings.MAX_DELIVER_ATTEMPTS,
                ack_wait=settings.ACK_WAIT_S,
            ),
        )

        await consumer(ctx, sub, msg_processed_kv, logger, job_orchestrator)
    finally:
        health_server.shutdown()
        if not nc.is_closed:
            await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(run_service())
