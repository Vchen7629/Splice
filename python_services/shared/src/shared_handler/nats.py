import asyncio
import contextlib
import json
from dataclasses import dataclass
from threading import Event
from typing import Any, AsyncGenerator, Awaitable, Callable

from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.errors import TimeoutError
from nats.js.client import JetStreamContext
from nats.js.errors import APIError, NotFoundError
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from shared_core import get_logger, sharedsettings
from shared_handler import ProcessJobMessage, UpscaleCompleteMsg, is_job_cancelled

from .exceptions import JobCancelledError
from .kv import check_already_processed, update_job_failed, update_job_stage
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
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass
        except Exception as e:
            logger.warning("keep_alive heartbeat failed", err=str(e))


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
        try:
            await task
        except asyncio.CancelledError:
            current_task = asyncio.current_task()
            if current_task is not None and current_task.cancelling():
                raise


@dataclass
class BaseNatsJSContext:
    nc: NATSClient
    js: JetStreamContext
    msg_processed_kv: KeyValue
    job_milestone_kv: KeyValue


@dataclass
class ProcessJobMsgContext(BaseNatsJSContext):
    metadata: ProcessJobMessage


@dataclass
class JobMsgContext(BaseNatsJSContext):
    service_name: str
    logger: BoundLogger


async def consumer(
    ctx: JobMsgContext,
    sub: JetStreamContext.PushSubscription,
    process_job_msg: Callable[[ProcessJobMsgContext, Event], Awaitable[None]],
    cleanup_job: Callable[[str], Awaitable[None]] | None = None,
) -> None:
    """Nats jetstream consumer that processes videos in the subscribed nats js"""
    async for msg in sub.messages:
        try:
            job_id = json.loads(msg.data)["job_id"]
        except Exception as e:
            ctx.logger.error("malformed nats msg, terminating", err=str(e))
            await msg.term()
            continue

        if job_id and await is_job_cancelled(ctx.job_milestone_kv, job_id):
            await msg.term()
            continue

        await _handle_consumer_message(ctx, msg, process_job_msg, cleanup_job)


async def _handle_consumer_message(
    ctx: JobMsgContext,
    msg: Msg,
    process_job_msg: Callable[[ProcessJobMsgContext, Event], Awaitable[None]],
    cleanup_job: Callable[[str], Awaitable[None]] | None = None,
) -> None:
    """TODO: docstring"""
    metadata: ProcessJobMessage | None = None
    needs_nak = False

    try:
        metadata = ProcessJobMessage.model_validate_json(msg.data.decode())
        job_id = metadata.job_id

        if await check_already_processed(ctx.msg_processed_kv, job_id):
            ctx.logger.debug("job already processed, skipping", job_id=job_id)
            await msg.ack()
            return

        await update_job_stage(
            ctx.job_milestone_kv, job_id, ctx.service_name, ctx.service_name
        )

        poll_interval = sharedsettings.ACK_WAIT_S / 3
        async with (
            keep_alive(msg, poll_interval, ctx.logger),
            check_cancel_event(
                ctx.job_milestone_kv, job_id, ctx.logger
            ) as cancel_event,
        ):
            processJobMsgCtx = ProcessJobMsgContext(
                ctx.nc, ctx.js, ctx.msg_processed_kv, ctx.job_milestone_kv, metadata
            )

            await process_job_msg(processJobMsgCtx, cancel_event)

        await ctx.msg_processed_kv.put(job_id, b"done")
        await msg.ack()
    except JobCancelledError as e:
        ctx.logger.debug("job cancelled during processing", err=str(e))
        await msg.ack()
    except Exception as e:
        ctx.logger.error("unexpected error processing job", err=str(e))
        if metadata is not None:
            try:
                await update_job_failed(
                    ctx.job_milestone_kv, metadata.job_id, str(e), ctx.service_name
                )
            except Exception:
                needs_nak = True
        if not needs_nak:
            await msg.ack()
    finally:
        if cleanup_job is not None and metadata is not None:
            await cleanup_job(metadata.job_id)
        if needs_nak:
            await msg.nak()


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
