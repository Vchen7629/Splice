from typing import Any
from shared_storage import queries
from pathlib import Path
from typing import Generator, Tuple
from testcontainers.core.container import DockerContainer
import requests
import pytest
import uuid
import time
import os

TEST_VIDEO_PATH = os.path.join(
    os.path.dirname(__file__), "..", "videos", "ForBiggerBlazes.mp4"
)
TEST_VIDEO_FILENAME = "ForBiggerBlazes.mp4"


@pytest.fixture(autouse=True)
def patch_temp_dir(tmp_path: Any, monkeypatch: Any) -> None:
    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))


def _wait_for_seaweedfs(
    host: str, master_port: int, filer_port: int, timeout: int = 60
) -> None:
    """Poll SeaweedFS master and filer HTTP endpoints until both are ready."""
    endpoints = [
        f"http://{host}:{master_port}/dir/status",
        f"http://{host}:{filer_port}/",
    ]
    for url in endpoints:
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                resp = requests.get(url, timeout=2)
                if resp.status_code < 500:
                    break
            except Exception:
                pass
            time.sleep(1)
        else:
            raise TimeoutError(f"SeaweedFS not ready at {url} after {timeout}s")


@pytest.fixture(scope="session")
def seaweedfs_url() -> Generator[str, None, None]:
    """Starts a SeaweedFS container and yields the filer base URL (http://host:8888)"""
    with (
        DockerContainer("chrislusf/seaweedfs")
        .with_command("server -dir=/data -master.port=9333 -volume.port=8080 -filer")
        .with_exposed_ports(9333, 8888)
    ) as container:
        host = container.get_container_host_ip()
        master_port = container.get_exposed_port(9333)
        filer_port = container.get_exposed_port(8888)
        _wait_for_seaweedfs(host, master_port, filer_port)
        yield f"http://{host}:{filer_port}"


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
