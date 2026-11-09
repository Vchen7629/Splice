from asyncio import (
    CancelledError,
    create_task,
    current_task as asyncio_current_task,
    sleep,
)
from contextlib import asynccontextmanager
from json import dumps, loads
from threading import Event
from typing import AsyncGenerator

from nats.js.errors import KeyNotFoundError, KeyWrongLastSequenceError
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from core import get_logger, settings


async def update_job_stage(
    job_milestone_kv: KeyValue, job_id: str, stage: str = settings.SERVICE_NAME
) -> None:
    """
    Writes the current processing stage to the job-milestones KV bucket

    Args:

    Exception:
        logs the error
    """
    logger = get_logger(settings.SERVICE_NAME)

    try:
        payload: dict[str, str] = {
            "state": "PROCESSING",
            "stage": stage,
        }
        await _advance_milestone(job_milestone_kv, job_id, payload)
    except Exception as e:
        logger.error("failed to update job-milestones stage", job_id=job_id, err=str(e))
        raise


async def update_job_failed(
    job_milestone_kv: KeyValue,
    job_id: str,
    error: str,
) -> None:
    """
    Writes FAILED with the underlying error message to the job-milestones KV bucket

    Raises:
        Exception: re-raised if the KV write fails, after logging
    """
    logger = get_logger(settings.SERVICE_NAME)

    payload = {"state": "FAILED", "error": error}
    try:
        await _advance_milestone(job_milestone_kv, job_id, payload)
    except Exception as e:
        logger.error(
            "failed to update job-milestones to failed", job_id=job_id, err=str(e)
        )
        raise


TERMINAL_MILESTONE_STATES = {"COMPLETE", "FAILED", "CANCELLED"}

MILESTONE_STAGE_ORDER = {
    "video-upscaling": 0,
    "video-recombiner": 1,
}


async def _advance_milestone(
    job_milestone_kv: KeyValue,
    job_id: str,
    payload: dict[str, str],
) -> None:
    """
    Writes payload to the job-milestones KV bucket only if it isn't behind the
    currently stored stage and the job isn't already in terminal state
    """
    new_state = payload.get("state", "")
    new_stage = payload.get("stage", "")
    new_ordinal = MILESTONE_STAGE_ORDER.get(new_stage, -1)
    status = dumps(payload).encode()

    while True:
        entry = await job_milestone_kv.get(job_id)

        current = loads(entry.value) if entry.value is not None else {}
        current_state = current.get("state", "")
        current_stage = current.get("stage", "")
        current_ordinal = MILESTONE_STAGE_ORDER.get(current_stage, -1)

        if current_state in TERMINAL_MILESTONE_STATES:
            return
        if (
            new_state not in TERMINAL_MILESTONE_STATES
            and current_ordinal >= new_ordinal
        ):
            return

        try:
            await job_milestone_kv.update(job_id, status, last=entry.revision)
        except KeyWrongLastSequenceError:
            continue  # revision changed concurrently; reread and compare again
        return


@asynccontextmanager
async def watch_for_cancel(
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
            await sleep(interval_s)
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

    task = create_task(_poll())
    try:
        yield cancel_event
    finally:
        task.cancel()
        try:
            await task
        except CancelledError:
            current_task = asyncio_current_task()
            if current_task is not None and current_task.cancelling():
                raise


async def is_job_cancelled(job_milestone_kv: KeyValue, job_id: str) -> bool:
    """Check the job_milestone_kv for the state of the job_id
    and returns true if the value state is CANCELLED, false otherwise"""
    try:
        entry = await job_milestone_kv.get(job_id)
    except KeyNotFoundError:
        return False
    current = loads(entry.value) if entry.value is not None else {}

    return current.get("state") == "CANCELLED"
