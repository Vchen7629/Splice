from pathlib import Path
from shared_storage.queries import upload_video
import os
import requests


def test_upload_chunks_happy_path(seaweedfs_url: str, chunk_files: list[str]) -> None:
    """All chunks are uploaded and returned URLs are reachable via GET"""
    job_id = "test-job-upload"

    storage_urls = [
        upload_video(
            f"{seaweedfs_url}/{job_id}/{os.path.basename(path)}",
            job_id,
            path,
            service_name="scene-detector",
        )
        for path in chunk_files
    ]

    assert len(storage_urls) == len(chunk_files)
    for url in storage_urls:
        assert url.startswith(f"{seaweedfs_url}/{job_id}/")
        resp = requests.get(url)
        assert resp.status_code == 200


def test_upload_single_chunk(seaweedfs_url: str, tmp_path: Path) -> None:
    """single chunk uploads and returns one URL"""
    job_id = "single-job"
    chunk = tmp_path / "video-Scene-001.mp4"
    chunk.write_bytes(b"single chunk")
    storage_url = f"{seaweedfs_url}/{job_id}/video-Scene-001.mp4"

    url = upload_video(storage_url, job_id, str(chunk), service_name="scene-detector")

    assert "video-Scene-001.mp4" in url
