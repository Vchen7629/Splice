from typing import AsyncGenerator, Awaitable, Callable
from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.kv import KeyValue
from nats.js.api import ConsumerConfig
from nats.errors import TimeoutError
from nats.js.errors import APIError
from nats.js.client import JetStreamContext
from shared_core import get_logger, settings
from shared_handler import UpscaleCompleteMsg
from .messages import VideoChunkMessage
import asyncio
import contextlib


@contextlib.asynccontextmanager
async def keep_alive(
    service_name: str, msg: Msg, interval: float
) -> AsyncGenerator[None, None]:
    """Periodically calls msg.in_progress() to extend Jetstream ack
    deadline while long-running work runs under 'msg'."""
    logger = get_logger(service_name)

    async def _heartbeat() -> None:
        while True:
            await asyncio.sleep(interval)
            await msg.in_progress()

    task = asyncio.create_task(_heartbeat())
    try:
        yield
    finally:
        try:
            task.cancel()
        except Exception as e:  # keep-alive is best-effort
            logger.warning("failed to extend ack deadline", err=str(e))
        with contextlib.suppress(asyncio.CancelledError):
            await task


async def consumer(
    nc: NATSClient,
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    job_milestone_kv: KeyValue,
    sub_subject: str,
    durable_name: str,
    queue_name: str,
    process_msg: Callable[
        [NATSClient, JetStreamContext, KeyValue, KeyValue, Msg], Awaitable[None]
    ],
) -> None:
    """Nats jetstream consumer that subscribes to subject to process videos"""
    sub = await js.subscribe(
        subject=sub_subject,
        durable=durable_name,
        queue=queue_name,
        config=ConsumerConfig(
            max_deliver=settings.MAX_DELIVER_ATTEMPTS, ack_wait=settings.ACK_WAIT_S
        ),
    )

    async for msg in sub.messages:
        await process_msg(nc, js, msg_processed_kv, job_milestone_kv, msg)


async def publisher(
    js: JetStreamContext,
    msg: VideoChunkMessage | UpscaleCompleteMsg,
    subject: str,
    service_name: str,
) -> None:
    """
    Publishes message to nats jetstream

    Args:
        js: the jetstream context with connection info for publishing
        msg: the actual data we are publishing to the broker
        subject: the jetstream subject we want to publish to
        service_name: the service name to log with

    Raises:
        TimeoutError: when publishing times out, logs and raises
        APIError: when an jetstream api error is recieved when trying
        to publish, logs and raises
    """
    logger = get_logger(service_name)

    try:
        await js.publish(subject=subject, payload=msg.model_dump_json().encode())
        logger.debug("pub msg to nats jetstream successfully")
    except TimeoutError as e:
        logger.error("timed out publishing msg", job_id=msg.job_id, err=str(e))
        raise
    except APIError as e:
        logger.error("jetstream error publishing msg", job_id=msg.job_id, err=str(e))
        raise
