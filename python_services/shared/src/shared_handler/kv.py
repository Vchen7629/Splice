from nats.js.kv import KeyValue
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from nats.js.errors import KeyNotFoundError
from shared_core.logging import get_logger
from shared_core.settings import settings
import json
import nats.js.errors as js_errors


async def connect_kv(js: JetStreamContext, kv_name: str) -> KeyValue:
    """
    Connect to an existing jetstream kv

    Args:
        js: jetstreamContext server we are connecting to
        kv_name: the kv we are trying to connect to

    Returns:
        the Jetstream KeyValue connection

    Raises:
        RuntimeError if the Jetstream KV isnt found
    """
    try:
        job_status_kv = await js.key_value(kv_name)

        return job_status_kv
    except js_errors.NotFoundError:
        raise RuntimeError(
            f"{kv_name} KV bucket not found, check video-status is running"
        )


async def create_kv(js: JetStreamContext, bucket_name: str) -> KeyValue:
    """
    Create a new Jetstream KV

    Args:
        js: jetstreamContext server we creating the new KV on
        kv_name: the kv we are trying to create

    Returns:
        the Jetstream KeyValue connection

    Raises:
        RuntimeError if the a API error happens with jetstream
    """
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


async def update_job_stage(
    job_milestone_kv: KeyValue,
    job_id: str,
    stage: str,
    service_name: str,
) -> None:
    """
    Writes the current processing stage to the job-milestones KV bucket

    Args:

    Exception:
        logs the error
    """
    logger = get_logger(service_name)

    try:
        payload: dict[str, str | int] = {"state": "PROCESSING", "stage": stage}
        status = json.dumps(payload).encode()
        await job_milestone_kv.put(job_id, status)
    except Exception as e:
        logger.error("failed to update job-milestones stage", job_id=job_id, err=str(e))
        raise


async def update_job_failed(
    job_milestone_kv: KeyValue,
    job_id: str,
    error: str,
    service_name: str,
) -> None:
    """
    Writes FAILED with the underlying error message to the job-milestones KV bucket

    Raises:
        Exception: re-raised if the KV write fails, after logging
    """
    logger = get_logger(service_name)

    payload = {"state": "FAILED", "error": error}
    status = json.dumps(payload).encode()
    try:
        await job_milestone_kv.put(job_id, status)
    except Exception as e:
        logger.error(
            "failed to update job-milestones to failed", job_id=job_id, err=str(e)
        )
        raise
