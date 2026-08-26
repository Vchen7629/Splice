from pathlib import Path
from typing import Generator, Tuple
import requests
import pytest
import uuid
import os

TEST_VIDEO_PATH = os.path.join(
    os.path.dirname(__file__), "..", "videos", "ForBiggerBlazes.mp4"
)
TEST_VIDEO_FILENAME = "ForBiggerBlazes.mp4"


@pytest.fixture
def seeded_video(seaweedfs_url: str) -> Generator[Tuple[str, str], None, None]:
    """Seeds ForBiggerBlazes.mp4 into SeaweedFS and yields (job_id, storage_url)"""
    job_id = str(uuid.uuid4())
    storage_url = f"{seaweedfs_url}/{job_id}/{TEST_VIDEO_FILENAME}"

    with open(TEST_VIDEO_PATH, "rb") as f:
        response = requests.put(
            storage_url,
            data=f,
            headers={"Content-Type": "application/octet-stream"},
            timeout=20,
        )
    response.raise_for_status()

    yield job_id, storage_url


@pytest.fixture
def fake_base_url() -> str:
    return "http://fake:8888"


@pytest.fixture
def chunk_files(tmp_path: Path) -> list[str]:
    """Creates a set of fake .mp4 chunk files in tmp_path"""
    chunks = []
    for i in range(3):
        chunk = tmp_path / f"video-Scene-{i + 1:03d}.mp4"
        chunk.write_bytes(b"fake chunk content")
        chunks.append(str(chunk))
    return chunks


@pytest.fixture
def single_video_chunk(tmp_path: Path) -> str:
    chunk = tmp_path / "chunk.mp4"
    chunk.write_bytes(b"data")
    return str(chunk)
