from typing import Any
from unittest.mock import AsyncMock
from nats.js.api import KeyValueConfig
from nats.js.client import JetStreamContext
from shared_core import get_logger
from shared_handler import consumer, keep_alive
import pytest
import asyncio


@pytest.mark.asyncio
async def test_consumer_calls_process_msg_for_published_message(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    import json

    """Verifies consumer receives a message and calls process_msg"""
    nc, js = js_context

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-status-1")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-job-status-1")
    )
    processed = asyncio.Event()

    async def _process_msg(*args: Any, **kwargs: Any) -> None:
        processed.set()

    process_msg = AsyncMock(side_effect=_process_msg)

    task = asyncio.create_task(
        consumer(
            nc,
            js,
            kv,
            job_status_kv,
            "jobs.video.scene-split",
            "test-consumer",
            "test-consumer",
            process_msg,
        )
    )
    try:
        await nc.publish(
            "jobs.video.scene-split", json.dumps({"job_id": "job-1"}).encode()
        )
        await asyncio.wait_for(processed.wait(), timeout=5)
    finally:
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert process_msg.call_count == 1


@pytest.mark.asyncio
async def test_keep_alive_sets_cancel_event_when_nats_msg_pubbed(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    from nats.aio.msg import Msg

    nc, _ = js_context
    msg = AsyncMock(spec=Msg)
    logger = get_logger("test")

    async with keep_alive(nc, msg, "job-1", interval=10, logger=logger) as cancel_event:
        await nc.publish("cancel.job-1", b"")
        await asyncio.wait_for(asyncio.to_thread(cancel_event.wait), timeout=5)

    assert cancel_event.is_set()
