from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from nats.js.api import ConsumerConfig
from ..core.logging import logger
from ..core.settings import settings
from ..processing.job import process_job
from .messages import SceneSplitMessage
from .publisher import scene_video_chunks
from nats.js.client import JetStreamContext


async def raw_videos(js: JetStreamContext, kv: KeyValue) -> None:
    """Nats jetstream consumer that subscribes to subject to process videos"""
    sub = await js.subscribe(
        subject=settings.SCENE_SPLIT_SUBJECT,
        durable=settings.NATS_SUB_QUEUE_NAME,
        queue=settings.NATS_SUB_QUEUE_NAME,
        config=ConsumerConfig(
            max_deliver=settings.MAX_DELIVER_ATTEMPTS, ack_wait=settings.ACK_WAIT_S
        ),
    )

    async for msg in sub.messages:
        try:
            metadata = SceneSplitMessage.model_validate_json(msg.data.decode())
            if await _is_already_processed(kv, metadata.job_id):
                logger.debug("job already processed, skipping", job_id=metadata.job_id)
                await msg.ack()
                continue
            chunk_messages = await process_job(metadata)
            await scene_video_chunks(js, chunk_messages)
            await kv.put(metadata.job_id, b"done")
            await msg.ack()
        except Exception as e:
            logger.error("unexpected error processing job", err=str(e))
            await msg.nak()


async def _is_already_processed(kv: KeyValue, job_id: str) -> bool:
    """Checks if the job_id exists in the scene-split-processed so it doesnt reprocess"""
    try:
        await kv.get(job_id)
        return True
    except KeyNotFoundError:
        return False
