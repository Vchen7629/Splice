from pathlib import Path
from unittest.mock import patch, MagicMock
from src.storage.queries import upload_video_chunks
import requests
import pytest


@pytest.fixture
def fake_base_url(monkeypatch: pytest.MonkeyPatch) -> str:
    url = "http://fake:8888"
    monkeypatch.setattr("src.storage.queries.settings.BASE_STORAGE_URL", url)
    return url


def test_raises_file_not_found(fake_base_url: str, tmp_path: Path) -> None:
    """Raises FileNotFoundError when a chunk path does not exist on disk"""
    with pytest.raises(FileNotFoundError):
        upload_video_chunks("job-1", [str(tmp_path / "missing.mp4")])


@pytest.mark.parametrize("status_code", [400, 404, 500, 503])
def test_raises_on_http_error(
    fake_base_url: str, single_video_chunk: str, status_code: int
) -> None:
    """Raises HTTPError when SeaweedFS returns 4xx/5xx on upload"""
    mock_response = MagicMock()
    mock_response.status_code = status_code
    mock_response.raise_for_status.side_effect = requests.HTTPError(
        response=mock_response
    )

    with (
        patch("src.storage.queries.requests.put", return_value=mock_response),
        pytest.raises(requests.HTTPError),
    ):
        upload_video_chunks("job-1", [single_video_chunk])


def test_raises_on_connection_error(
    fake_base_url: str, single_video_chunk: str
) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable during upload"""
    with (
        patch("src.storage.queries.requests.put", side_effect=requests.ConnectionError),
        pytest.raises(requests.ConnectionError),
    ):
        upload_video_chunks("job-1", [single_video_chunk])


def test_returns_correct_storage_urls(fake_base_url: str, tmp_path: Path) -> None:
    """Returns list of SeaweedFS URLs matching {base}/{job_id}/{filename}"""
    job_id = "job-abc"
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
        f"{fake_base_url}/{job_id}/chunk-001.mp4",
        f"{fake_base_url}/{job_id}/chunk-002.mp4",
    ]
