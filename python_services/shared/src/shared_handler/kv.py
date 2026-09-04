from nats.js.kv import KeyValue
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from nats.js.errors import KeyNotFoundError
from shared_core import get_logger, settings
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


async def is_job_cancelled(job_milestone_kv: KeyValue, job_id: str) -> bool:
    """Check the job_milestone_kv for the state of the job_id
    and returns true if the value state is CANCELLED, false otherwise"""
    try:
        entry = await job_milestone_kv.get(job_id)
    except KeyNotFoundError:
        return False
    current = json.loads(entry.value) if entry.value is not None else {}

    return current.get("state") == "CANCELLED"


TERMINAL_MILESTONE_STATES = {"COMPLETE", "FAILED", "CANCELLED"}

MILESTONE_STAGE_ORDER = {
    "scene-detector": 0,
    "video-upscaling": 0,
    "video-recombiner": 1,
}


async def advance_milestone(
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
    status = json.dumps(payload).encode()

    while True:
        entry = await job_milestone_kv.get(job_id)

        current = json.loads(entry.value) if entry.value is not None else {}
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
        except js_errors.KeyWrongLastSequenceError:
            continue  # revision changed concurrently; reread and compare again
        return


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
        payload: dict[str, str] = {"state": "PROCESSING", "stage": stage}
        await advance_milestone(job_milestone_kv, job_id, payload)
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
    try:
        await advance_milestone(job_milestone_kv, job_id, payload)
    except Exception as e:
        logger.error(
            "failed to update job-milestones to failed", job_id=job_id, err=str(e)
        )
        raise
