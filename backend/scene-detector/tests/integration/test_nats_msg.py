from typing import Any
from unittest.mock import patch
from unittest.mock import AsyncMock
from nats.js import JetStreamContext
from nats.js.api import KeyValueConfig
from shared_handler.messages import ProcessJobMessage
from src.processing.nats_msg import process_msg
import json
import pytest
import uuid


@pytest.mark.asyncio
async def test_processes_published_message(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """Verifies process_msg parses the message and calls process_job with correct data"""
    nc, js = js_context

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-nats-msg-status-1")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-nats-msg-job-status-1")
    )

    job_id = str(uuid.uuid4())
    msg = AsyncMock()
    msg.data = json.dumps(
        {
            "job_id": job_id,
            "storage_url": "/fake/video.mp4",
            "source_resolution": "280p",
            "target_resolution": "480p",
        }
    ).encode()

    with patch(
        "src.processing.nats_msg.process_job", new_callable=AsyncMock, return_value=[]
    ) as mock_process:
        await process_msg(js, kv, job_status_kv, msg)

    mock_process.assert_called_once_with(
        ProcessJobMessage(
            job_id=job_id,
            storage_url="/fake/video.mp4",
            source_resolution="280p",
            target_resolution="480p",
        )
    )
    msg.ack.assert_called_once()


@pytest.mark.asyncio
async def test_skips_redelivered_message_for_already_processed_job(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """Verifies process_msg acks and skips when job_id already exists in KV"""
    nc, js = js_context

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-nats-msg-status-2")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-nats-msg-job-status-2")
    )
    await kv.put("job-already-done", b"done")

    msg = AsyncMock()
    msg.data = json.dumps(
        {
            "job_id": "job-already-done",
            "storage_url": "/fake/video.mp4",
            "source_resolution": "280p",
            "target_resolution": "480p",
        }
    ).encode()

    with patch(
        "src.processing.nats_msg.process_job", new_callable=AsyncMock, return_value=[]
    ) as mock_process:
        await process_msg(js, kv, job_status_kv, msg)

    mock_process.assert_not_called()
    msg.ack.assert_called_once()
