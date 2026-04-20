from nats.js.kv import KeyValue
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from nats.js.errors import KeyNotFoundError
from shared_core.logging import logger
from shared_core.settings import settings
import json
import nats.js.errors as js_errors


async def create_msg_processed_kv(bucket_name: str, js: JetStreamContext) -> KeyValue:
    try:
        msg_processed_kv = await js.create_key_value(
            config=KeyValueConfig(bucket=bucket_name, ttl=settings.KV_BUCKET_TTL_S)
        )

        return msg_processed_kv

    except js_errors.APIError as e:
        raise RuntimeError(f"failed to create {bucket_name} KV bucket: {e}")


async def check_already_processed(kv: KeyValue, job_id: str) -> bool:
    """Checks if the job_id exists in the kv so it doesnt reprocess"""
    try:
        await kv.get(job_id)
        return True

    except KeyNotFoundError:
        return False


async def update_job_status(job_status_kv: KeyValue, job_id: str, stage: str) -> None:
    """Writes PROCESSING for the stage to the job-status KV bucket"""
    try:
        status = json.dumps({"state": "PROCESSING", "stage": stage}).encode()
        await job_status_kv.put(job_id, status)
    except Exception as e:
        logger.error("failed to update job status stage", job_id=job_id, err=str(e))
