from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from nats.js.api import ConsumerConfig
from nats.aio.msg import Msg
from ..core.logging import logger
from ..core.settings import settings
from ..processing.job import process_job
from .messages import SceneSplitMessage
from .publisher import scene_video_chunks
from nats.js.client import JetStreamContext
import json


async def raw_videos(
    js: JetStreamContext, msg_processed_kv: KeyValue, job_status_kv: KeyValue
) -> None:
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
        await _process_msg(js, msg_processed_kv, job_status_kv, msg)


async def _process_msg(
    js: JetStreamContext, msg_processed_kv: KeyValue, job_status_kv: KeyValue, msg: Msg
) -> None:
    """Processes a single scene-split message"""
    try:
        metadata = SceneSplitMessage.model_validate_json(msg.data.decode())

        if await _is_already_processed(msg_processed_kv, metadata.job_id):
            logger.debug("job already processed, skipping", job_id=metadata.job_id)
            await msg.ack()
            return

        await _update_job_status(job_status_kv, metadata.job_id)

        chunk_messages = await process_job(metadata)

        await scene_video_chunks(js, chunk_messages)
        await msg_processed_kv.put(metadata.job_id, b"done")
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


async def _update_job_status(job_status_kv: KeyValue, job_id: str) -> None:
    """Writes PROCESSING:scene-detector stage to the job-status KV bucket"""
    try:
        status = json.dumps({"state": "PROCESSING", "stage": "scene-detector"}).encode()
        await job_status_kv.put(job_id, status)
    except Exception as e:
        logger.error("failed to update job status stage", job_id=job_id, err=str(e))
