from scenedetect import VideoOpenFailure
from unittest.mock import patch
from src.processing.job import process_job
from src.handler.messages import SceneSplitMessage, VideoChunkMessage
import pytest

METADATA = SceneSplitMessage(
    job_id="test-123",
    storage_url="http://fake:8888/test-123/video.mp4",
    target_resolution="480p",
)

FAKE_LOCAL_PATH = "/tmp/test-123/video.mp4"
FAKE_CHUNK_PATHS = [
    "/tmp/test-123/video-Scene-001.mp4",
    "/tmp/test-123/video-Scene-002.mp4",
]
FAKE_STORAGE_URLS = [
    "http://fake:8888/test-123/video-Scene-001.mp4",
    "http://fake:8888/test-123/video-Scene-002.mp4",
]


@pytest.mark.asyncio
@pytest.mark.parametrize("exc", [VideoOpenFailure, OSError])
async def test_split_exceptions_propagate(exc: type[Exception]) -> None:
    """VideoOpenFailure and OSError from split_into_chunks propagate out of process_job"""
    with (
        patch("src.processing.job.fetch_video", return_value=FAKE_LOCAL_PATH),
        patch("src.processing.job.split_into_chunks", side_effect=exc),
        pytest.raises(exc),
    ):
        await process_job(METADATA)


@pytest.mark.asyncio
async def test_uses_job_scoped_output_dir() -> None:
    """split_into_chunks is called with a job-scoped temp dir to prevent collisions"""
    with (
        patch("src.processing.job.fetch_video", return_value=FAKE_LOCAL_PATH),
        patch("src.processing.job.split_into_chunks", return_value=[]) as mock_split,
        patch("src.processing.job.upload_video_chunks", return_value=[]),
        patch("src.processing.job.shutil.rmtree"),
    ):
        await process_job(METADATA)

    mock_split.assert_called_once_with(
        FAKE_LOCAL_PATH, f"../temp/{METADATA.job_id}/chunks"
    )


@pytest.mark.asyncio
async def test_returns_chunk_messages_on_success() -> None:
    """Returns correct VideoChunkMessage list with SeaweedFS URLs"""
    with (
        patch("src.processing.job.fetch_video", return_value=FAKE_LOCAL_PATH),
        patch("src.processing.job.split_into_chunks", return_value=FAKE_CHUNK_PATHS),
        patch("src.processing.job.upload_video_chunks", return_value=FAKE_STORAGE_URLS),
        patch("src.processing.job.shutil.rmtree"),
    ):
        result = await process_job(METADATA)

    assert len(result) == 2
    assert all(isinstance(m, VideoChunkMessage) for m in result)
    assert result[0].chunk_index == 0
    assert result[1].chunk_index == 1
    assert result[0].job_id == METADATA.job_id
    assert result[0].storage_url == FAKE_STORAGE_URLS[0]
    assert result[1].storage_url == FAKE_STORAGE_URLS[1]
    assert result[0].total_chunks == 2
    assert result[1].total_chunks == 2


@pytest.mark.asyncio
async def test_cleans_up_temp_dir_after_upload() -> None:
    """Temp directory is removed after chunks are uploaded"""
    with (
        patch("src.processing.job.fetch_video", return_value=FAKE_LOCAL_PATH),
        patch("src.processing.job.split_into_chunks", return_value=FAKE_CHUNK_PATHS),
        patch("src.processing.job.upload_video_chunks", return_value=FAKE_STORAGE_URLS),
        patch("src.processing.job.shutil.rmtree") as mock_rmtree,
    ):
        await process_job(METADATA)

    mock_rmtree.assert_called_once_with(f"../temp/{METADATA.job_id}")
