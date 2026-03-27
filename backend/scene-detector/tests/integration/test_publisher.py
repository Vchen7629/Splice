from typing import Any
from nats.js.client import JetStreamContext
from src.nats.messages import VideoChunkMessage
from src.nats.publisher import scene_video_chunks
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
            storage_path="/fake/chunk-001.mp4",
            target_resolution="480p",
        ),
        VideoChunkMessage(
            job_id="1",
            chunk_index=1,
            total_chunks=2,
            storage_path="/fake/chunk-002.mp4",
            target_resolution="480p",
        ),
    ]

    await scene_video_chunks(js, MSGS)

    assert len(nats_video_chunks_subscriber) == 2
    assert nats_video_chunks_subscriber[0] == {
        "job_id": "1",
        "chunk_index": 0,
        "total_chunks": 2,
        "storage_path": "/fake/chunk-001.mp4",
        "target_resolution": "480p",
    }
    assert nats_video_chunks_subscriber[1] == {
        "job_id": "1",
        "chunk_index": 1,
        "total_chunks": 2,
        "storage_path": "/fake/chunk-002.mp4",
        "target_resolution": "480p",
    }
