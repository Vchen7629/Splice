from typing import Any
from typing import Generator
from typing import AsyncGenerator
from src.nats.messages import VideoChunkMessage
from src.nats.publisher import scene_video_chunks
import nats
import pytest

@pytest.mark.asyncio
async def test_publishes_all_messages_with_correct_payload(
    nats_url: Generator[str, None],
    nats_video_chunks_subscriber: AsyncGenerator[list[Any], None],
    monkeypatch
) -> None:
    monkeypatch.setattr("src.nats.publisher.settings.NATS_URL", nats_url)
    nc = await nats.connect(nats_url)
    js = nc.jetstream()

    MSGS = [
        VideoChunkMessage(job_id="1", chunk_index=0, storage_path="/fake/chunk-001.mp4"),                                                                        
        VideoChunkMessage(job_id="1", chunk_index=1, storage_path="/fake/chunk-002.mp4"),                                                                        
    ]
        

    await scene_video_chunks(js, MSGS)
    await nc.flush()

    assert len(nats_video_chunks_subscriber) == 2
    assert nats_video_chunks_subscriber[0] == {"job_id": "1", "chunk_index": 0, "storage_path": "/fake/chunk-001.mp4"}                                                    
    assert nats_video_chunks_subscriber[1] == {"job_id": "1", "chunk_index": 1, "storage_path": "/fake/chunk-002.mp4"} 

    await nc.close()