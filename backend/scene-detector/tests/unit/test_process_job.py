from scenedetect import VideoOpenFailure
from unittest.mock import patch
from src.processing.job import process_job
from src.nats.messages import SceneSplitMessage, VideoChunkMessage
import os
import pytest
import tempfile

METADATA = SceneSplitMessage(
    job_id="test-123", storage_path="/fake/video.mp4", target_resolution="480p"
)


@pytest.mark.asyncio
async def test_catches_video_open_failure() -> None:
    """Video open failure triggers when video exists but scenedetect is unable to open it"""
    with tempfile.NamedTemporaryFile(suffix=".mp4", delete=False) as f:
        f.write(b"Not a video")
        tmp_path = f.name
    try:
        with pytest.raises(VideoOpenFailure):
            await process_job(
                SceneSplitMessage(
                    job_id="1",
                    storage_path=tmp_path,
                    target_resolution=METADATA.target_resolution,
                )
            )
    finally:
        os.unlink(tmp_path)


@pytest.mark.asyncio
async def test_catches_video_not_found() -> None:
    """OSError raised when video path does not exist"""
    with pytest.raises(OSError):
        await process_job(
            SceneSplitMessage(
                job_id="1",
                storage_path="/nonexistent/video.mp4",
                target_resolution=METADATA.target_resolution,
            )
        )


@pytest.mark.asyncio
async def test_returns_chunk_messages_on_success() -> None:
    """Returns correct VideoChunkMessage list when split succeeds"""
    chunk_paths = ["/tmp/video-Scene-001.mp4", "/tmp/video-Scene-002.mp4"]

    with patch("src.processing.job.split_into_chunks", return_value=chunk_paths):
        result = await process_job(METADATA)

    assert len(result) == 2
    assert all(isinstance(m, VideoChunkMessage) for m in result)
    assert result[0].chunk_index == 0
    assert result[1].chunk_index == 1
    assert result[0].job_id == METADATA.job_id
    assert result[0].storage_path == chunk_paths[0]
    assert result[1].storage_path == chunk_paths[1]
