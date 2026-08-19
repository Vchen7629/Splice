from typing import Any
from unittest.mock import AsyncMock
from nats.js.api import KeyValueConfig
from nats.js.client import JetStreamContext
from shared_handler.nats import consumer
import pytest
import asyncio


@pytest.mark.asyncio
async def test_calls_process_msg_for_published_message(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """Verifies consumer receives a message and calls process_msg"""
    nc, js = js_context

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-status-1")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-job-status-1")
    )
    process_msg = AsyncMock()

    task = asyncio.create_task(
        consumer(
            js,
            kv,
            job_status_kv,
            "jobs.video.scene-split",
            "test-consumer",
            "test-consumer",
            process_msg,
        )
    )
    await nc.publish("jobs.video.scene-split", b"test-payload")
    await asyncio.sleep(0.5)
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass

    assert process_msg.call_count == 1
