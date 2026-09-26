import asyncio
import json
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest
from nats.aio.client import Client as NATSClient
from nats.js.api import KeyValueConfig
from nats.js.client import JetStreamContext
from structlog.stdlib import BoundLogger

from shared_handler import JobMsgContext, check_cancel_event, consumer, nats_connect

MOCK_LOGGER = MagicMock(spec=BoundLogger)


@pytest.mark.asyncio
async def test_nats_connect_returns_connected_clients(
    nats_url: str, monkeypatch: Any
) -> None:
    monkeypatch.setattr("shared_handler.nats.sharedsettings.NATS_URL", nats_url)

    nc, js = await nats_connect(service_name="scene-detector")

    assert isinstance(nc, NATSClient)
    assert isinstance(js, JetStreamContext)
    assert nc.is_connected
    await nc.close()


@pytest.mark.asyncio
async def test_consumer_calls_process_job_msg_for_published_message(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """Verifies consumer receives a message and calls process_job_msg"""
    nc, js = js_context

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-status-1")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-consumer-job-status-1")
    )
    await job_status_kv.put(
        "job-1", json.dumps({"state": "PROCESSING", "stage": "upload"}).encode()
    )
    processed = asyncio.Event()

    async def _process_job_msg(*args: Any, **kwargs: Any) -> None:
        processed.set()

    process_job_msg = AsyncMock(side_effect=_process_job_msg)

    sub = await js.subscribe(
        subject="jobs.video.scene-split",
        durable="test-consumer",
        queue="test-consumer",
    )

    ctx = JobMsgContext(nc, js, kv, job_status_kv, "test-service", MOCK_LOGGER)
    task = asyncio.create_task(consumer(ctx, sub, process_job_msg))
    try:
        await nc.publish(
            "jobs.video.scene-split",
            json.dumps(
                {
                    "job_id": "job-1",
                    "storage_url": "/fake/video.mp4",
                    "source_resolution": "1080p",
                    "target_resolution": "480p",
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

    assert process_job_msg.call_count == 1


@pytest.mark.asyncio
async def test_check_cancel_event_sets_cancel_event_when_kv_marks_cancelled(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    _, js = js_context
    job_milestone_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-check-cancel-event-1")
    )
    await job_milestone_kv.put("job-1", json.dumps({"state": "PROCESSING"}).encode())

    async with check_cancel_event(
        job_milestone_kv, "job-1", MOCK_LOGGER, interval_s=0.05
    ) as cancel_event:
        await job_milestone_kv.put("job-1", json.dumps({"state": "CANCELLED"}).encode())
        await asyncio.wait_for(asyncio.to_thread(cancel_event.wait), timeout=5)

    assert cancel_event.is_set()


@pytest.mark.asyncio
async def test_check_cancel_event_stays_unset_when_kv_never_cancelled(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    _, js = js_context
    job_milestone_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-check-cancel-event-2")
    )
    await job_milestone_kv.put("job-2", json.dumps({"state": "PROCESSING"}).encode())

    async with check_cancel_event(
        job_milestone_kv, "job-2", MOCK_LOGGER, interval_s=0.05
    ) as cancel_event:
        await asyncio.sleep(0.2)

    assert not cancel_event.is_set()
