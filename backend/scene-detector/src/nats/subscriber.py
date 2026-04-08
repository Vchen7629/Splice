from ..core.logging import logger
from ..core.settings import settings
from ..processing.job import process_job
from .messages import SceneSplitMessage
from .publisher import scene_video_chunks
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
            metadata = SceneSplitMessage.model_validate_json(msg.data.decode())
            chunk_messages = await process_job(metadata)
            await scene_video_chunks(js, chunk_messages)
            await msg.ack()
        except Exception as e:
            logger.error("unexpected error processing job", err=str(e))
            await msg.nak()
