from scenedetect import VideoOpenFailure
from unittest.mock import patch
from unittest.mock import AsyncMock
from src.processing.job import process_job
from src.nats.messages import SceneSplitMessage
import os
import pytest
import tempfile

METADATA = SceneSplitMessage(job_id="test-123", storage_path="/fake/video.mp4")


@pytest.mark.asyncio
async def test_catches_video_open_failure() -> None:
    """Video open failure triggers when video exists but scenedetect is unable to split"""
    with tempfile.NamedTemporaryFile(suffix=".mp4", delete=False) as f:
        f.write(b"Not a video")
        tmp_path = f.name
    try:
        with pytest.raises(VideoOpenFailure):
            await process_job(
                SceneSplitMessage(job_id="1", storage_path=tmp_path), js=AsyncMock()
            )
    finally:
        os.unlink(tmp_path)


@pytest.mark.asyncio
async def test_catches_video_not_found() -> None:
    """if video doesnt exit it will raise OS error"""
    with pytest.raises(OSError):
        await process_job(
            SceneSplitMessage(job_id="1", storage_path="idk"), js=AsyncMock()
        )


@pytest.mark.asyncio
async def test_publishes_chunk_messages_on_sucess() -> None:
    """Happy path test"""
    chunk_paths = ["/tmp/video-Scene-001.mp4", "/tmp/video-Scene-002.mp4"]
    mock_js = AsyncMock()

    with (
        patch("src.processing.job.split_into_chunks", return_value=chunk_paths),
        patch(
            "src.processing.job.scene_video_chunks", new_callable=AsyncMock
        ) as mock_publish,
    ):
        await process_job(METADATA, js=mock_js)

    mock_publish.assert_called_once()
    messages = mock_publish.call_args[0][1]
    assert len(messages) == 2
    assert messages[0].chunk_index == 0
    assert messages[1].chunk_index == 1
    assert messages[0].job_id == METADATA.job_id
