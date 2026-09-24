import asyncio

from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from shared_core import get_logger
from shared_handler import (
    JobCancelledError,
    ProcessJobMessage,
    check_already_processed,
    check_cancel_event,
    keep_alive,
    publisher,
    update_job_failed,
    update_job_stage,
)
from shared_util import ProgressReporter

from ..core.settings import settings
from ..processing.job import process_job

logger = get_logger("scene-detector")


async def process_msg(
    nc: NATSClient,
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    job_milestone_kv: KeyValue,
    msg: Msg,
) -> None:
    """Processes a single scene-split message"""
    metadata: ProcessJobMessage | None = None
    service_name = settings.SERVICE_NAME

    try:
        metadata = ProcessJobMessage.model_validate_json(msg.data.decode())
        job_id = metadata.job_id

        if await check_already_processed(msg_processed_kv, job_id):
            logger.debug("job already processed, skipping", job_id=job_id)
            await msg.ack()
            return

        await update_job_stage(
            job_milestone_kv,
            job_id,
            service_name,
            service_name,
        )

        loop = asyncio.get_event_loop()
        reporter = ProgressReporter(nc, job_id, loop, service_name)

        poll_interval = settings.ACK_WAIT_S / 3
        async with (
            keep_alive(msg, poll_interval, logger),
            check_cancel_event(job_milestone_kv, job_id, logger) as cancel_event,
        ):
            chunk_messages = await process_job(cancel_event, metadata, reporter)
            await reporter.flush()

            for chunk_msg in chunk_messages:
                await publisher(js, chunk_msg, settings.PUB_SUBJECT, service_name)

            await msg_processed_kv.put(metadata.job_id, b"done")
    except JobCancelledError as e:
        logger.debug("job cancelled during processing", err=str(e))
        await msg.ack()
        return
    except Exception as e:
        logger.error("unexpected error processing job", err=str(e))
        if metadata is not None:
            try:
                await update_job_failed(
                    job_milestone_kv, metadata.job_id, str(e), service_name
                )
            except Exception:
                await msg.nak()
                return
        await msg.ack()
        return

    await msg.ack()
