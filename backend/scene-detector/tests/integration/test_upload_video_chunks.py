from pathlib import Path
from src.storage.queries import upload_video_chunks
import requests
import pytest


def test_upload_chunks_happy_path(
    seaweedfs_url: str, chunk_files: list[str], monkeypatch: pytest.MonkeyPatch
) -> None:
    """All chunks are uploaded and returned URLs are reachable via GET"""
    monkeypatch.setattr("src.storage.queries.settings.BASE_STORAGE_URL", seaweedfs_url)
    job_id = "test-job-upload"

    storage_urls = upload_video_chunks(job_id, chunk_files)

    assert len(storage_urls) == len(chunk_files)
    for url in storage_urls:
        assert url.startswith(f"{seaweedfs_url}/{job_id}/")
        resp = requests.get(url)
        assert resp.status_code == 200


def test_upload_single_chunk(
    seaweedfs_url: str, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """single chunk uploads and returns one URL"""
    monkeypatch.setattr("src.storage.queries.settings.BASE_STORAGE_URL", seaweedfs_url)
    chunk = tmp_path / "video-Scene-001.mp4"
    chunk.write_bytes(b"single chunk")

    storage_urls = upload_video_chunks("single-job", [str(chunk)])

    assert len(storage_urls) == 1
    assert "video-Scene-001.mp4" in storage_urls[0]
