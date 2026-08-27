from typing import Any
from nats.js.client import JetStreamContext
from shared_handler import VideoChunkMessage, publisher
import asyncio
import pytest


@pytest.mark.asyncio
async def test_publishes_all_messages_with_correct_payload(
    js_context: tuple[Any, JetStreamContext],
    nats_video_chunks_subscriber: list[Any],
) -> None:
    _, js = js_context
    MSGS = [
        VideoChunkMessage(
            job_id="1",
            chunk_index=0,
            total_chunks=2,
            storage_url="/fake/chunk-001.mp4",
            target_resolution="480p",
        ),
        VideoChunkMessage(
            job_id="1",
            chunk_index=1,
            total_chunks=2,
            storage_url="/fake/chunk-002.mp4",
            target_resolution="480p",
        ),
    ]

    for msg in MSGS:
        await publisher(js, msg, "jobs.video.chunks", service_name="scene-detector")

    async def _wait_for_delivery() -> None:
        while len(nats_video_chunks_subscriber) < len(MSGS):
            await asyncio.sleep(0.05)

    await asyncio.wait_for(_wait_for_delivery(), timeout=5)

    assert len(nats_video_chunks_subscriber) == 2
    assert nats_video_chunks_subscriber[0] == {
        "job_id": "1",
        "chunk_index": 0,
        "total_chunks": 2,
        "storage_url": "/fake/chunk-001.mp4",
        "target_resolution": "480p",
    }
    assert nats_video_chunks_subscriber[1] == {
        "job_id": "1",
        "chunk_index": 1,
        "total_chunks": 2,
        "storage_url": "/fake/chunk-002.mp4",
        "target_resolution": "480p",
    }
