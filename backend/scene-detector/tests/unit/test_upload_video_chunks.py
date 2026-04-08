from unittest.mock import patch
from unittest.mock import MagicMock
from pathlib import Path
from src.storage.queries import upload_video_chunks
import requests
import pytest


def test_raises_file_not_found(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Raises FileNotFoundError when a chunk path does not exist on disk"""
    monkeypatch.setattr(
        "src.storage.queries.settings.BASE_STORAGE_URL", "http://fake:8888"
    )
    with pytest.raises(FileNotFoundError):
        upload_video_chunks("job-1", [str(tmp_path / "missing.mp4")])


@pytest.mark.parametrize("status_code", [400, 404, 500, 503])
def test_raises_on_http_error(
    tmp_path: Path, status_code: int, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Raises HTTPError when SeaweedFS returns 4xx/5xx on upload"""
    monkeypatch.setattr(
        "src.storage.queries.settings.BASE_STORAGE_URL", "http://fake:8888"
    )
    chunk = tmp_path / "chunk.mp4"
    chunk.write_bytes(b"data")

    mock_response = MagicMock()
    mock_response.status_code = status_code
    mock_response.raise_for_status.side_effect = requests.HTTPError(
        response=mock_response
    )

    with (
        patch("src.storage.queries.requests.put", return_value=mock_response),
        pytest.raises(requests.HTTPError),
    ):
        upload_video_chunks("job-1", [str(chunk)])


def test_raises_on_connection_error(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable during upload"""
    monkeypatch.setattr(
        "src.storage.queries.settings.BASE_STORAGE_URL", "http://fake:8888"
    )
    chunk = tmp_path / "chunk.mp4"
    chunk.write_bytes(b"data")

    with (
        patch("src.storage.queries.requests.put", side_effect=requests.ConnectionError),
        pytest.raises(requests.ConnectionError),
    ):
        upload_video_chunks("job-1", [str(chunk)])


def test_returns_correct_storage_urls(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Returns list of SeaweedFS URLs matching {base}/{job_id}/{filename}"""
    base_url = "http://fake:8888"
    job_id = "job-abc"
    monkeypatch.setattr("src.storage.queries.settings.BASE_STORAGE_URL", base_url)

    chunks = []
    for name in ["chunk-001.mp4", "chunk-002.mp4"]:
        f = tmp_path / name
        f.write_bytes(b"data")
        chunks.append(str(f))

    mock_response = MagicMock()
    mock_response.raise_for_status.return_value = None

    with patch("src.storage.queries.requests.put", return_value=mock_response):
        urls = upload_video_chunks(job_id, chunks)

    assert urls == [
        f"{base_url}/{job_id}/chunk-001.mp4",
        f"{base_url}/{job_id}/chunk-002.mp4",
    ]
