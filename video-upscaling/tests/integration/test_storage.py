from pathlib import Path
from unittest.mock import MagicMock

import pytest
import requests

from core import VideoNotFoundErr, settings
from storage import check_storage_health, fetch_video, upload_video

BAD_URLS = ["http://localhost:1", "http://doesnotexist.invalid"]


def test_check_storage_health_succeeds(
    seaweedfs_url: str, mock_logger: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Passes without raising when the SeaweedFS filer is reachable"""
    monkeypatch.setattr(settings, "BASE_STORAGE_URL", seaweedfs_url)
    check_storage_health(mock_logger)


@pytest.mark.parametrize("bad_url", BAD_URLS)
def test_check_storage_health_raises_on_connection_error(
    bad_url: str, mock_logger: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable"""
    monkeypatch.setattr(settings, "BASE_STORAGE_URL", bad_url)
    with pytest.raises(requests.ConnectionError):
        check_storage_health(mock_logger)


def test_fetch_video_downloads_file(
    seeded_video: tuple[str, str], tmp_path: Path
) -> None:
    """Video is downloaded to {TEMP_ROOT}/input/{job_id}/{filename}"""
    job_id, storage_url = seeded_video
    local_path = fetch_video(storage_url)

    assert local_path == f"{tmp_path}/input/{job_id}/one_frame.mp4"
    assert Path(local_path).stat().st_size > 0


def test_fetch_video_raises_on_404(seaweedfs_url: str) -> None:
    """Raises VideoNotFoundErr when the video does not exist in storage"""
    with pytest.raises(VideoNotFoundErr):
        fetch_video(f"{seaweedfs_url}/nonexistent-job/missing.mp4")


@pytest.mark.parametrize("bad_url", BAD_URLS)
def test_fetch_video_raises_on_connection_error(bad_url: str) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable"""
    with pytest.raises(requests.ConnectionError):
        fetch_video(f"{bad_url}/job/video.mp4")


def test_upload_video_is_fetchable(seaweedfs_url: str, local_video: str) -> None:
    """Uploaded video is readable back from the returned url"""
    storage_url = f"{seaweedfs_url}/job-upload/video.mp4"

    url = upload_video(storage_url, "job-upload", local_video)

    assert url == storage_url
    resp = requests.get(url, timeout=20)
    assert resp.status_code == 200
    assert resp.content == b"data"
