from typing import Tuple
from src.storage.queries import fetch_video
import requests
import pytest
import os


def test_fetch_video_downloads_file(seeded_video: Tuple[str, str]) -> None:
    """File is downloaded and saved locally with the correct filename"""
    job_id, storage_url = seeded_video
    local_path = fetch_video(storage_url)

    assert os.path.exists(local_path)
    assert os.path.getsize(local_path) > 0
    assert os.path.basename(local_path) == "ForBiggerBlazes.mp4"


def test_fetch_video_creates_directory(seeded_video: Tuple[str, str]) -> None:
    """Destination directory is created if it does not already exist"""
    job_id, storage_url = seeded_video
    local_path = fetch_video(storage_url)

    assert os.path.isdir(os.path.dirname(local_path))


def test_fetch_video_path_namespaced_by_job_id(seeded_video: Tuple[str, str]) -> None:
    """Local path includes the job_id segment to prevent collisions across jobs"""
    job_id, storage_url = seeded_video
    local_path = fetch_video(storage_url)

    assert job_id in local_path


def test_fetch_video_raises_on_404(seaweedfs_url: str) -> None:
    """Raises HTTPError when the video does not exist in storage"""
    missing_url = f"{seaweedfs_url}/nonexistent-job/missing.mp4"
    with pytest.raises(requests.HTTPError):
        fetch_video(missing_url)


@pytest.mark.parametrize(
    "bad_url",
    [
        "http://localhost:1/job/video.mp4",
        "http://doesnotexist.invalid/job/video.mp4",
    ],
)
def test_fetch_video_raises_on_connection_error(bad_url: str) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable"""
    with pytest.raises(requests.ConnectionError):
        fetch_video(bad_url)
