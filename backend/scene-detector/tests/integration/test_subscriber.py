from typing import Any
from unittest.mock import patch
from nats.js.client import JetStreamContext
from nats.js.api import KeyValueConfig
from src.nats.subscriber import raw_videos
from src.nats.messages import SceneSplitMessage
import json
import pytest
import asyncio
import uuid


@pytest.mark.asyncio
async def test_processes_published_message(
    js_context: tuple[Any, JetStreamContext], monkeypatch: Any
) -> None:
    """Verifies subscriber receives a message and calls process_job with correct data"""
    nc, js = js_context
    monkeypatch.setattr(
        "src.nats.subscriber.settings.SCENE_SPLIT_SUBJECT", "jobs.video.scene-split"
    )
    monkeypatch.setattr(
        "src.nats.subscriber.settings.NATS_SUB_QUEUE_NAME", "scene-detector-workers"
    )
    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-scene-split-status-1")
    )

    job_id = str(uuid.uuid4())
    payload = json.dumps(
        {
            "job_id": job_id,
            "storage_url": "/fake/video.mp4",
            "target_resolution": "480p",
        }
    ).encode()
    received: list[Any] = []

    async def fake_process_job(metadata: Any) -> list[Any]:
        received.append(metadata)
        return []

    with patch("src.nats.subscriber.process_job", side_effect=fake_process_job):
        task = asyncio.create_task(raw_videos(js, kv))
        await nc.publish("jobs.video.scene-split", payload)
        await asyncio.sleep(0.5)  # let the subscriber process the message
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert len(received) == 1
    assert received[0] == SceneSplitMessage(
        job_id=job_id, storage_url="/fake/video.mp4", target_resolution="480p"
    )


@pytest.mark.asyncio
async def test_skips_redelivered_message_for_already_processed_job(
    js_context: tuple[Any, JetStreamContext], monkeypatch: Any
) -> None:
    """Verifies subscriber acks and skips processing when job_id already exists in KV"""
    nc, js = js_context
    monkeypatch.setattr(
        "src.nats.subscriber.settings.SCENE_SPLIT_SUBJECT", "jobs.video.scene-split"
    )
    monkeypatch.setattr(
        "src.nats.subscriber.settings.NATS_SUB_QUEUE_NAME",
        "scene-detector-workers-idempotency",
    )
    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-scene-split-status-2")
    )
    await kv.put("job-already-done", b"done")

    payload = json.dumps(
        {
            "job_id": "job-already-done",
            "storage_url": "/fake/video.mp4",
            "target_resolution": "480p",
        }
    ).encode()
    process_calls: list[Any] = []

    async def fake_process_job(metadata: Any) -> list[Any]:
        process_calls.append(metadata)
        return []

    with patch("src.nats.subscriber.process_job", side_effect=fake_process_job):
        task = asyncio.create_task(raw_videos(js, kv))
        await nc.publish("jobs.video.scene-split", payload)
        await asyncio.sleep(0.5)
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert len(process_calls) == 0
