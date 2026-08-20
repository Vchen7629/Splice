from shared_handler.messages import ProcessJobMessage
from nats.js.kv import KeyValue
from nats.aio.msg import Msg
from shared_core.logging import get_logger
from shared_handler.kv import update_job_status
from shared_handler.kv import update_job_failed
from shared_handler.kv import check_already_processed
from shared_handler.nats import publisher
from ..core.settings import settings
from ..processing.job import process_job
from nats.js.client import JetStreamContext

logger = get_logger("scene-detector")


async def process_msg(
    js: JetStreamContext, msg_processed_kv: KeyValue, job_status_kv: KeyValue, msg: Msg
) -> None:
    """Processes a single scene-split message"""
    try:
        metadata = ProcessJobMessage.model_validate_json(msg.data.decode())

        if await check_already_processed(msg_processed_kv, metadata.job_id):
            logger.debug("job already processed, skipping", job_id=metadata.job_id)
            await msg.ack()
            return

        await update_job_status(
            job_status_kv, metadata.job_id, settings.SERVICE_NAME, settings.SERVICE_NAME
        )

        chunk_messages = await process_job(metadata)

        for chunk_msg in chunk_messages:
            await publisher(js, chunk_msg, settings.PUB_SUBJECT, settings.SERVICE_NAME)

        await msg_processed_kv.put(metadata.job_id, b"done")
    except Exception as e:
        logger.error("unexpected error processing job", err=str(e))
        if "metadata" in locals():
            try:
                await update_job_failed(
                    job_status_kv, metadata.job_id, str(e), settings.SERVICE_NAME
                )
            except Exception:
                await msg.nak()
                return
        await msg.ack()
        return

    await msg.ack()
