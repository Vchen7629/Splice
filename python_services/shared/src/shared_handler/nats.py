from typing import AsyncGenerator, Awaitable, Callable
from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.kv import KeyValue
from nats.js.api import ConsumerConfig
from nats.errors import TimeoutError
from nats.js.errors import APIError
from nats.js.client import JetStreamContext
from structlog.stdlib import BoundLogger
from threading import Event
from shared_core import get_logger, settings
from shared_handler import UpscaleCompleteMsg, is_job_cancelled
from .messages import VideoChunkMessage
import asyncio
import contextlib
import json


@contextlib.asynccontextmanager
async def keep_alive(
    msg: Msg, interval: float, logger: BoundLogger
) -> AsyncGenerator[None, None]:
    """Periodically calls msg.in_progress() to extend the Jetstream ack deadline,
    and subscribes to cancel.{job_id} for the duration of the work, setting
    cancel_event when a cancel broadcast arrives so long-running loops can check it."""

    async def _heartbeat() -> None:
        while True:
            await asyncio.sleep(interval)
            await msg.in_progress()

    task = asyncio.create_task(_heartbeat())
    try:
        yield task
    finally:
        try:
            task.cancel()
        except Exception as e:  # keep-alive is best-effort
            logger.warning("failed to extend ack deadline", err=str(e))
        with contextlib.suppress(asyncio.CancelledError):
            await task


@contextlib.asynccontextmanager
async def check_cancel_event(
    nc: NATSClient, job_id: str, logger: BoundLogger
) -> AsyncGenerator[None, None]:
    """"""
    cancel_event = Event()

    async def _on_cancel(_: Msg) -> None:
        cancel_event.set()

    cancel_sub = await nc.subscribe(f"cancel.{job_id}", cb=_on_cancel)
    try:
        yield cancel_event
    finally:
        try:
            await cancel_sub.unsubscribe()
        except Exception as e:  # keep-alive is best-effort
            logger.warning("failed to unsubscribe from cancel subject", err=str(e))


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
        job_id = json.loads(msg.data)["job_id"]
        if job_id and await is_job_cancelled(job_milestone_kv, job_id):
            await msg.term()
            continue

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
