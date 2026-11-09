import asyncio
import json
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest
from nats.js.api import KeyValueConfig
from nats.js.client import JetStreamContext

from core import settings
from msgbus import JobContext, consumer


@pytest.mark.asyncio
async def test_consumer_calls_job_orchestrator_for_published_message(
    js_context: tuple[Any, JetStreamContext], mock_logger: MagicMock
) -> None:
    """Verifies consumer receives a message and calls job_orchestrator"""
    nc, js = js_context

    processed_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-status-1")
    )
    milestone_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-job-status-1")
    )
    await milestone_kv.put(
        "job-1", json.dumps({"state": "PROCESSING", "stage": "upload"}).encode()
    )
    processed = asyncio.Event()

    async def _job_orchestrator(*args: Any, **kwargs: Any) -> None:
        processed.set()

    job_orchestrator = AsyncMock(side_effect=_job_orchestrator)

    sub = await js.subscribe(
        subject=settings.SUB_SUBJECT,
        durable="test-consumer",
        queue="test-consumer",
    )

    ctx = JobContext(nc, js, milestone_kv)
    task = asyncio.create_task(
        consumer(ctx, sub, processed_kv, mock_logger, job_orchestrator)
    )
    try:
        await nc.publish(
            settings.SUB_SUBJECT,
            json.dumps(
                {
                    "job_id": "job-1",
                    "storage_url": "/fake/video.mp4",
                    "source_resolution": "1080p",
                    "target_resolution": "480p",
                    "operation": "upscale",
                }
            ).encode(),
        )
        await asyncio.wait_for(processed.wait(), timeout=5)
    finally:
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert job_orchestrator.call_count == 1
