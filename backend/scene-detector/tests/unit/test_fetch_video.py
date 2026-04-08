from unittest.mock import patch
from unittest.mock import MagicMock
from pathlib import Path
from src.storage.queries import fetch_video
import src.storage.queries as queries
import requests
import pytest


@pytest.mark.parametrize("status_code", [500, 502, 503])
def test_fetch_video_raises_on_server_error(status_code: int) -> None:
    """Raises HTTPError when SeaweedFS returns a 5xx response"""
    mock_response = MagicMock()
    mock_response.status_code = status_code
    mock_response.raise_for_status.side_effect = requests.HTTPError(
        response=mock_response
    )

    with (
        patch("src.storage.queries.requests.get", return_value=mock_response),
        pytest.raises(requests.HTTPError),
    ):
        fetch_video("http://fake/job-id/video.mp4")


def test_fetch_video_writes_correct_content(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """File written locally contains the exact bytes from the response"""
    fake_content = b"fake video bytes"
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.raise_for_status.return_value = None
    mock_response.content = fake_content

    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))
    with patch("src.storage.queries.requests.get", return_value=mock_response):
        local_path = fetch_video("http://fake/job-123/video.mp4")

    with open(local_path, "rb") as f:
        assert f.read() == fake_content
