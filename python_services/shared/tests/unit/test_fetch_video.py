import io
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
import requests

from shared_storage import fetch_video, queries


@pytest.mark.parametrize("status_code", [500, 502, 503])
def test_fetch_video_raises_on_server_error(status_code: int) -> None:
    """Raises HTTPError when SeaweedFS returns a 5xx response"""
    mock_response = MagicMock()
    mock_response.__enter__.return_value = mock_response
    mock_response.status_code = status_code
    mock_response.raise_for_status.side_effect = requests.HTTPError(
        response=mock_response
    )

    with (
        patch("shared_storage.queries.requests.get", return_value=mock_response),
        pytest.raises(requests.HTTPError),
    ):
        fetch_video("http://fake/job-id/video.mp4", service_name="scene-detector")

    mock_response.__exit__.assert_called_once()


def test_fetch_video_removes_partial_file_on_copy_failure(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A failure mid-download leaves no partial file behind"""
    mock_response = MagicMock()
    mock_response.__enter__.return_value = mock_response
    mock_response.raise_for_status.return_value = None
    mock_response.raw.read.side_effect = [b"partial", requests.ConnectionError("cut")]

    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))
    with (
        patch("shared_storage.queries.requests.get", return_value=mock_response),
        pytest.raises(requests.ConnectionError),
    ):
        fetch_video("http://fake/job-123/video.mp4", service_name="scene-detector")

    assert not (tmp_path / "job-123" / "video.mp4").exists()


def test_fetch_video_writes_correct_content(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """File written locally contains the exact bytes from the response"""
    fake_content = b"fake video bytes"
    mock_response = MagicMock()
    mock_response.__enter__.return_value = mock_response
    mock_response.status_code = 200
    mock_response.raise_for_status.return_value = None
    mock_response.raw = io.BytesIO(fake_content)

    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))
    with patch("shared_storage.queries.requests.get", return_value=mock_response):
        local_path = fetch_video(
            "http://fake/job-123/video.mp4", service_name="scene-detector"
        )

    with open(local_path, "rb") as f:
        assert f.read() == fake_content
