import io
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
import requests

from core import VideoNotFoundErr
from storage import check_storage_health, fetch_video, upload_video

FAKE_URL = "http://fake/job-123/video.mp4"


def _response(status_code: int = 200) -> MagicMock:
    """Mock requests response usable as a context manager, raises HTTPError on 4xx/5xx"""
    resp = MagicMock()
    resp.__enter__.return_value = resp
    resp.status_code = status_code
    if status_code >= 400:
        resp.raise_for_status.side_effect = requests.HTTPError(response=resp)
    return resp


def test_fetch_video_writes_content(tmp_path: Path) -> None:
    """Saves the response bytes to {TEMP_ROOT}/input/{job_id}/{filename}"""
    resp = _response()
    resp.raw = io.BytesIO(b"fake video bytes")

    with patch("storage.queries.requests.get", return_value=resp):
        local_path = fetch_video(FAKE_URL)

    assert local_path == f"{tmp_path}/input/job-123/video.mp4"
    assert Path(local_path).read_bytes() == b"fake video bytes"


@pytest.mark.parametrize("status_code", [500, 502, 503])
def test_fetch_video_raises_on_server_error(status_code: int) -> None:
    """Raises HTTPError on 5xx and closes the response"""
    resp = _response(status_code)

    with (
        patch("storage.queries.requests.get", return_value=resp),
        pytest.raises(requests.HTTPError),
    ):
        fetch_video(FAKE_URL)

    resp.__exit__.assert_called_once()


def test_fetch_video_raises_not_found_on_404() -> None:
    """A 404 is raised as VideoNotFoundErr, not a generic HTTPError"""
    with (
        patch("storage.queries.requests.get", return_value=_response(404)),
        pytest.raises(VideoNotFoundErr),
    ):
        fetch_video(FAKE_URL)


def test_fetch_video_removes_partial_file_on_copy_failure(tmp_path: Path) -> None:
    """A failure mid-download leaves no partial file behind"""
    resp = _response()
    resp.raw.read.side_effect = [b"partial", requests.ConnectionError("cut")]

    with (
        patch("storage.queries.requests.get", return_value=resp),
        pytest.raises(requests.ConnectionError),
    ):
        fetch_video(FAKE_URL)

    assert list((tmp_path / "input" / "job-123").iterdir()) == []


@pytest.mark.parametrize("failure", ["http_error", "copy_error"])
def test_fetch_video_keeps_existing_file_on_failure(
    failure: str, tmp_path: Path
) -> None:
    """A failed fetch must not delete or corrupt an already-downloaded video"""
    existing = tmp_path / "input" / "job-123" / "video.mp4"
    existing.parent.mkdir(parents=True)
    existing.write_bytes(b"original bytes")

    if failure == "http_error":
        resp, expected = _response(500), requests.HTTPError
    else:
        resp, expected = _response(), requests.ConnectionError
        resp.raw.read.side_effect = [b"partial", requests.ConnectionError("cut")]

    with (
        patch("storage.queries.requests.get", return_value=resp),
        pytest.raises(expected),
    ):
        fetch_video(FAKE_URL)

    assert existing.read_bytes() == b"original bytes"
    assert list(existing.parent.iterdir()) == [existing]


def test_upload_video_raises_file_not_found(tmp_path: Path) -> None:
    """Raises FileNotFoundError when the local video does not exist"""
    with pytest.raises(FileNotFoundError):
        upload_video(FAKE_URL, "job-123", str(tmp_path / "missing.mp4"))


@pytest.mark.parametrize("status_code", [400, 404, 500, 503])
def test_upload_video_raises_on_http_error(local_video: str, status_code: int) -> None:
    """Raises HTTPError when SeaweedFS returns 4xx/5xx"""
    with (
        patch("storage.queries.requests.put", return_value=_response(status_code)),
        pytest.raises(requests.HTTPError),
    ):
        upload_video(FAKE_URL, "job-123", local_video)


def test_upload_video_raises_on_connection_error(local_video: str) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable"""
    with (
        patch("storage.queries.requests.put", side_effect=requests.ConnectionError),
        pytest.raises(requests.ConnectionError),
    ):
        upload_video(FAKE_URL, "job-123", local_video)


def test_upload_video_returns_url_and_sends_processed_ttl(local_video: str) -> None:
    """Returns the storage url and uploads with the 3d processed-video TTL"""
    with patch("storage.queries.requests.put", return_value=_response()) as mock_put:
        url = upload_video(FAKE_URL, "job-123", local_video)

    assert url == FAKE_URL
    assert mock_put.call_args.kwargs["params"] == {"ttl": "3d"}


@pytest.mark.parametrize("status_code", [500, 502, 503])
def test_check_storage_health_raises_on_server_error(
    status_code: int, mock_logger: MagicMock
) -> None:
    """Raises HTTPError when the SeaweedFS filer returns 5xx"""
    with (
        patch("storage.check_health.requests.get", return_value=_response(status_code)),
        pytest.raises(requests.HTTPError),
    ):
        check_storage_health(mock_logger)
