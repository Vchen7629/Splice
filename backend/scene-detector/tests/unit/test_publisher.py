from typing import Any
from unittest.mock import AsyncMock
from nats.errors import TimeoutError
from nats.js.errors import APIError
from nats.js.client import JetStreamContext
from src.nats.publisher import scene_video_chunks
from src.nats.messages import VideoChunkMessage
import pytest


@pytest.mark.asyncio
@pytest.mark.parametrize(argnames="exc", argvalues=[APIError(), TimeoutError()])
async def test_raises_on_publish_failure(exc: Any) -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.publish.side_effect = exc

    with pytest.raises(type(exc)):
        await scene_video_chunks(
            mock_js,
            [
                VideoChunkMessage(
                    job_id="1",
                    chunk_index=0,
                    total_chunks=1,
                    storage_path="/fake/path.mp4",
                    target_resolution="480p",
                )
            ],
        )


@pytest.mark.asyncio
async def test_calls_publish_per_msg() -> None:
    """publish should be called as many times as the amount of items in the input list"""
    mock_js = AsyncMock(spec=JetStreamContext)

    await scene_video_chunks(
        mock_js,
        [
            VideoChunkMessage(
                job_id="1",
                chunk_index=0,
                total_chunks=2,
                storage_path="/fake/path.mp4",
                target_resolution="480p",
            ),
            VideoChunkMessage(
                job_id="1",
                chunk_index=0,
                total_chunks=2,
                storage_path="/fake/path.mp4",
                target_resolution="480p",
            ),
        ],
    )

    assert mock_js.publish.call_count == 2
