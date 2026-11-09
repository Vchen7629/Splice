import uuid
from pathlib import Path

import pytest
import requests


@pytest.fixture
def seeded_video(seaweedfs_url: str, one_frame_video: Path) -> tuple[str, str]:
    """Uploads one_frame_video to SeaweedFS under a fresh job_id, returns (job_id, storage_url)"""
    job_id = str(uuid.uuid4())
    storage_url = f"{seaweedfs_url}/{job_id}/{one_frame_video.name}"
    with open(one_frame_video, "rb") as f:
        requests.put(
            storage_url,
            data=f,
            headers={"Content-Type": "application/octet-stream"},
            timeout=20,
        ).raise_for_status()
    return job_id, storage_url


@pytest.fixture
def local_video(tmp_path: Path) -> str:
    """Small placeholder file for upload tests"""
    video = tmp_path / "video.mp4"
    video.write_bytes(b"data")
    return str(video)
