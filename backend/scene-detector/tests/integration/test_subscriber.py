from typing import Any
from unittest.mock import patch
from nats.js.kv import KeyValue
from nats.js.api import KeyValueConfig
from nats.js.client import JetStreamContext
from src.handler.subscriber import raw_videos
from src.handler.messages import SceneSplitMessage
import json
import pytest
import asyncio
import uuid


async def _run_subscriber(
    nc: Any,
    js: JetStreamContext,
    kv: KeyValue,
    job_status_kv: KeyValue,
    payload: bytes,
) -> list[Any]:
    """
    Launch raw_videos as a task, pub one msg, wait, then cancel
    returrns all processed jobs as a side effect of the process_job
    """
    processed_job: list[Any] = []

    async def fake_process_job(metadata: Any) -> list[Any]:
        processed_job.append(metadata)
        return []

    with patch("src.handler.subscriber.process_job", side_effect=fake_process_job):
        task = asyncio.create_task(raw_videos(js, kv, job_status_kv))
        await nc.publish("jobs.video.scene-split", payload)
        await asyncio.sleep(0.5)  # let the subscriber process the message
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    return processed_job


@pytest.mark.asyncio
async def test_processes_published_message(
    js_context: tuple[Any, JetStreamContext], monkeypatch: Any
) -> None:
    """Verifies subscriber receives a message and calls process_job with correct data"""
    nc, js = js_context

    monkeypatch.setattr(
        "src.handler.subscriber.settings.SCENE_SPLIT_SUBJECT", "jobs.video.scene-split"
    )
    monkeypatch.setattr(
        "src.handler.subscriber.settings.NATS_SUB_QUEUE_NAME", "scene-detector-workers"
    )

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-scene-split-status-1")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-job-status-sub-1")
    )

    job_id = str(uuid.uuid4())
    payload = json.dumps(
        {
            "job_id": job_id,
            "storage_url": "/fake/video.mp4",
            "target_resolution": "480p",
        }
    ).encode()

    recieved = await _run_subscriber(nc, js, kv, job_status_kv, payload)

    assert len(recieved) == 1
    assert recieved[0] == SceneSplitMessage(
        job_id=job_id, storage_url="/fake/video.mp4", target_resolution="480p"
    )


@pytest.mark.asyncio
async def test_skips_redelivered_message_for_already_processed_job(
    js_context: tuple[Any, JetStreamContext], monkeypatch: Any
) -> None:
    """Verifies subscriber acks and skips processing when job_id already exists in KV"""
    nc, js = js_context

    monkeypatch.setattr(
        "src.handler.subscriber.settings.SCENE_SPLIT_SUBJECT", "jobs.video.scene-split"
    )
    monkeypatch.setattr(
        "src.handler.subscriber.settings.NATS_SUB_QUEUE_NAME",
        "scene-detector-workers-idempotency",
    )

    kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-scene-split-status-2")
    )
    job_status_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-job-status-sub-2")
    )
    await kv.put("job-already-done", b"done")

    payload = json.dumps(
        {
            "job_id": "job-already-done",
            "storage_url": "/fake/video.mp4",
            "target_resolution": "480p",
        }
    ).encode()

    process_calls = await _run_subscriber(nc, js, kv, job_status_kv, payload)

    assert len(process_calls) == 0
