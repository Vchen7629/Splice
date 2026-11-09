from asyncio import CancelledError, create_task, sleep, to_thread
from contextlib import asynccontextmanager
from dataclasses import dataclass
from json import loads
from shutil import rmtree
from subprocess import TimeoutExpired
from threading import Event
from typing import Any, AsyncGenerator, Awaitable, Callable

from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js import JetStreamContext
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from core import JobCancelledError, VideoNotFoundErr, input_dir, output_dir, settings

from .kv import check_already_processed
from .messages import ProcessJobMessage
from .milestone import (
    is_job_cancelled,
    update_job_failed,
    update_job_stage,
    watch_for_cancel,
)


@dataclass
class JobContext:
    nc: NATSClient
    js: JetStreamContext
    milestone_kv: KeyValue


async def consumer(
    ctx: JobContext,
    sub: JetStreamContext.PushSubscription,
    msg_processed_kv: KeyValue,
    logger: BoundLogger,
    job_orchestrator: Callable[
        [JobContext, ProcessJobMessage, Event, BoundLogger], Awaitable[None]
    ],
) -> None:
    """Nats jetstream consumer that processes videos in the subscribed nats js"""
    async for processMsg in sub.messages:
        try:
            job_id = loads(processMsg.data)["job_id"]
        except Exception as e:
            logger.error("malformed nats msg, terminating", err=str(e))
            await processMsg.term()
            continue

        if job_id and await is_job_cancelled(ctx.milestone_kv, job_id):
            await processMsg.term()
            continue

        await _handle_consumer_message(
            ctx, msg_processed_kv, processMsg, logger, job_orchestrator
        )


async def _handle_consumer_message(
    ctx: JobContext,
    msg_processed_kv: KeyValue,
    processMsg: Msg,
    logger: BoundLogger,
    job_orchestrator: Callable[
        [JobContext, ProcessJobMessage, Event, BoundLogger], Awaitable[None]
    ],
) -> None:
    """TODO: docstring"""
    job: ProcessJobMessage | None = None
    needs_nak = False

    try:
        job = ProcessJobMessage.model_validate_json(processMsg.data.decode())

        if await check_already_processed(msg_processed_kv, job.job_id):
            logger.debug("job already processed, skipping", job_id=job.job_id)
            await processMsg.ack()
            return

        await update_job_stage(ctx.milestone_kv, job.job_id)

        poll_interval = settings.ACK_WAIT_S / 3
        async with (
            keep_alive(processMsg, poll_interval, logger),
            watch_for_cancel(ctx.milestone_kv, job.job_id, logger) as cancel_event,
        ):
            await job_orchestrator(ctx, job, cancel_event, logger)

        await msg_processed_kv.put(job.job_id, b"done")
        await processMsg.ack()
    except JobCancelledError as e:
        logger.debug("job cancelled during processing", err=str(e))
        await processMsg.ack()
    except TimeoutExpired as e:
        if processMsg.metadata.num_delivered < settings.MAX_DELIVER_ATTEMPTS:
            logger.error("timed out processing job, naking for retry", err=str(e))
            needs_nak = True
        else:
            logger.error("timed out on final delivery, failing job", err=str(e))
            needs_nak = not await _fail_job(ctx, job, processMsg, str(e))
    except VideoNotFoundErr as e:
        logger.error("storage artifact missing or expired", err=str(e))
        needs_nak = not await _fail_job(
            ctx, job, processMsg, "video missing or expired from storage"
        )
    except Exception as e:
        logger.error("unexpected error processing job", err=str(e))
        needs_nak = not await _fail_job(ctx, job, processMsg, str(e))
    finally:
        if job is not None:
            await _cleanup_job(job.job_id, logger)
        if needs_nak:
            await processMsg.nak()


async def _cleanup_job(
    job_id: str, logger: BoundLogger, retries: int = 3, delay_s: float = 1.0
) -> None:
    """TODO: docstring"""
    for attempt in range(1, retries + 1):
        try:
            for d in (output_dir(job_id), input_dir(job_id)):
                await to_thread(rmtree, d, ignore_errors=True)

            return
        except OSError as e:
            if attempt == retries:
                logger.error(
                    "failed to clean up temp paths after retries",
                    job_id=job_id,
                    attempts=attempt,
                    err=str(e),
                )
                return
            await sleep(delay_s)


@asynccontextmanager
async def keep_alive(
    msg: Msg, interval: float, logger: BoundLogger
) -> AsyncGenerator[Any, None]:
    """Periodically calls msg.in_progress() to extend the Jetstream ack deadline"""

    async def _heartbeat() -> None:
        while True:
            await sleep(interval)
            await msg.in_progress()

    task = create_task(_heartbeat())
    try:
        yield task
    finally:
        task.cancel()
        try:
            await task
        except CancelledError:
            pass
        except Exception as e:
            logger.warning("keep_alive heartbeat failed", err=str(e))


async def _fail_job(
    ctx: JobContext,
    job: ProcessJobMessage | None,
    processMsg: Msg,
    err: str,
) -> bool:
    """marks job FAILED and acks. returns false if KV write failed so caller should nak and redeliver"""
    try:
        if job is not None:
            await update_job_failed(ctx.milestone_kv, job.job_id, err)
    except Exception:
        return False
    await processMsg.ack()
    return True
