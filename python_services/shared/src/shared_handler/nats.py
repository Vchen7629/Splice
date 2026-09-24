import asyncio
import contextlib
import json
from threading import Event
from typing import Any, AsyncGenerator, Awaitable, Callable

from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.errors import TimeoutError
from nats.js.api import ConsumerConfig
from nats.js.client import JetStreamContext
from nats.js.errors import APIError, NotFoundError
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from shared_core import get_logger, sharedsettings
from shared_handler import UpscaleCompleteMsg, is_job_cancelled

from .messages import VideoChunkMessage


async def nats_connect(service_name: str) -> tuple[NATSClient, JetStreamContext]:
    """nats connection and jetstream context required for pub/sub"""
    nats_url = sharedsettings.NATS_URL
    logger = get_logger(service_name)

    async def _on_reconnect() -> None:
        logger.debug("reconnected to nats")

    async def _on_disconnect() -> None:
        logger.warning("disconnected from nats")

    async def _on_error(err: Exception) -> None:
        logger.error("error connecting to nats", err=str(err))

    nats_client = NATSClient()
    await nats_client.connect(
        nats_url,
        max_reconnect_attempts=sharedsettings.MAX_RECONNECT_ATTEMPT,
        reconnect_time_wait=sharedsettings.RECONNECT_TIME_WAIT_S,
        reconnected_cb=_on_reconnect,
        disconnected_cb=_on_disconnect,
        error_cb=_on_error,
    )

    jetstream_client: JetStreamContext = nats_client.jetstream()

    return nats_client, jetstream_client


@contextlib.asynccontextmanager
async def keep_alive(
    msg: Msg, interval: float, logger: BoundLogger
) -> AsyncGenerator[Any, None]:
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
    job_milestone_kv: KeyValue,
    job_id: str,
    logger: BoundLogger,
    interval_s: float = 2.0,
) -> AsyncGenerator[Event, None]:
    """periodically poll the job_milestone_kv to check if the job for job_id is cancelled to let the
    services know they should stop processing"""
    cancel_event = Event()

    async def _poll() -> None:
        while True:
            await asyncio.sleep(interval_s)
            try:
                if await is_job_cancelled(job_milestone_kv, job_id):
                    cancel_event.set()
                    return
            except Exception as e:
                logger.warning(
                    "failed to poll job cancellation state, retrying",
                    job_id=job_id,
                    err=str(e),
                )

    task = asyncio.create_task(_poll())
    try:
        yield cancel_event
    finally:
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await task


async def consumer(
    logger: BoundLogger,
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
            max_deliver=sharedsettings.MAX_DELIVER_ATTEMPTS,
            ack_wait=sharedsettings.ACK_WAIT_S,
        ),
    )

    async for msg in sub.messages:
        try:
            job_id = json.loads(msg.data)["job_id"]
        except Exception as e:
            logger.error("malformed nats msg, terminating", err=str(e))
            await msg.term()
            continue

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


async def check_js_stream_exists(js: JetStreamContext, subject_name: str) -> None:
    """
    Check if a js stream exists using the subject name. Used before trying to
    connect to the stream in order to fail early

    Args:
        js: the jetstream context connection
        subject_name: the stream subject name we are checking

    Raises:
        RuntimeError if the jetstream stream doesnt exist
    """
    try:
        await js.find_stream_name_by_subject(subject_name)
    except NotFoundError:
        raise RuntimeError(f"No stream found for `{subject_name}`")
