from ..core.logging import logger
from ..core.settings import settings
from ..processing.job import process_job
from nats.js.client import JetStreamContext


async def raw_videos(js: JetStreamContext) -> None:
    """Nats jetstream consumer that subscribes to subject to process videos"""
    sub = await js.subscribe(
        subject=settings.SCENE_SPLIT_SUBJECT,
        durable=settings.NATS_SUB_QUEUE_NAME,
        queue=settings.NATS_SUB_QUEUE_NAME,
    )
    async for msg in sub.messages:
        try:
            await process_job(msg.data.decode(), js)
            await msg.ack()
        except Exception as e:
            logger.error("video split error", err=str(e))
            await msg.nak()
