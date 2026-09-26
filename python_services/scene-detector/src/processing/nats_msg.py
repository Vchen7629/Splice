import asyncio
from threading import Event

from shared_core import get_logger
from shared_handler import ProcessJobMsgContext, publisher
from shared_util import ProgressReporter

from ..core.settings import settings
from ..processing.job import process_job

SERVICE_NAME = settings.SERVICE_NAME
logger = get_logger(SERVICE_NAME)


async def process_job_msg(ctx: ProcessJobMsgContext, cancel_event: Event) -> None:
    """TODO: docstring"""
    loop = asyncio.get_event_loop()
    reporter = ProgressReporter(ctx.nc, ctx.metadata.job_id, loop, SERVICE_NAME)

    chunk_messages = await process_job(cancel_event, ctx.metadata, reporter)
    await reporter.flush()

    for chunk_msg in chunk_messages:
        await publisher(ctx.js, chunk_msg, settings.PUB_SUBJECT, SERVICE_NAME)
