from typing import Awaitable, Callable, Protocol

from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue

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


class ServiceSettings(Protocol):
    SERVICE_NAME: str
    HTTP_PORT: int
    SUB_SUBJECT: str
    PUB_SUBJECT: str
    SUB_QUEUE_NAME: str


async def run_service(
    settings: ServiceSettings,
    processed_kv_name: str,
    process_msg_handler: Callable[
        [NATSClient, JetStreamContext, KeyValue, KeyValue, Msg], Awaitable[None]
    ],
) -> None:
    """Handles starting the service (transcoder and video-upscaling)
    It starts the health server, connects to nats jetstream, connects to job-milestones kv and creates
    the processed kv and starts the consumer poll loop. ALso handles graceful shutdown

    Args:
        settings: service settings (service name, http port, etc)
        processed_kv_name: the kv name we are creating for the service to mark jobids as procesed
        process_msg_handler: per-msg handler passed through consumer
    """
    logger = get_logger(settings.SERVICE_NAME)

    check_storage_health(settings.SERVICE_NAME)
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect(settings.SERVICE_NAME)

    try:
        await check_js_stream_exists(js, settings.SUB_SUBJECT)
        await check_js_stream_exists(js, settings.PUB_SUBJECT)

        job_milestone_kv = await connect_kv(js, "job-milestones")
        msg_processed_kv = await create_kv(js, processed_kv_name)

        await consumer(
            logger,
            nc,
            js,
            msg_processed_kv,
            job_milestone_kv,
            settings.SUB_SUBJECT,
            settings.SUB_QUEUE_NAME,
            settings.SUB_QUEUE_NAME,
            process_msg=process_msg_handler,
        )
    finally:
        health_server.shutdown()
        if not nc.is_closed:
            await nc.drain()
