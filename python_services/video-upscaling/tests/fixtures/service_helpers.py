from pathlib import Path
from typing import Any
from typing import Generator
from unittest.mock import patch
from unittest.mock import MagicMock
from testcontainers.core.container import DockerContainer
import time
import pytest
import requests
import subprocess

TEST_VIDEO = Path(__file__).parent.parent / "fixtures" / "testvideo.mp4"


def _wait_for_seaweedfs(
    host: str, master_port: int, filer_port: int, timeout: int = 60
) -> None:
    for url in [
        f"http://{host}:{master_port}/dir/status",
        f"http://{host}:{filer_port}/",
    ]:
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                if requests.get(url, timeout=2).status_code < 500:
                    break
            except Exception:
                pass
            time.sleep(1)
        else:
            raise TimeoutError(f"SeaweedFS not ready at {url}")


@pytest.fixture(scope="session")
def seaweedfs_url() -> Generator[str, None, None]:
    with (
        DockerContainer("chrislusf/seaweedfs")
        .with_command("server -dir=/data -master.port=9333 -volume.port=8080 -filer")
        .with_exposed_ports(9333, 8888)
    ) as container:
        host = container.get_container_host_ip()
        master_port = int(container.get_exposed_port(9333))
        filer_port = int(container.get_exposed_port(8888))
        _wait_for_seaweedfs(host, master_port, filer_port)
        yield f"http://{host}:{filer_port}"


@pytest.fixture(scope="session")
def uploaded_test_video(
    seaweedfs_url: str, tmp_path_factory: pytest.TempPathFactory
) -> str:
    """Generates a tiny 1-frame mp4, uploads to SeaweedFS, returns the storage URL."""
    tiny = tmp_path_factory.mktemp("video") / "tiny.mp4"
    subprocess.run(
        [
            "ffmpeg",
            "-y",
            "-f",
            "lavfi",
            "-i",
            "color=c=blue:size=128x72:rate=1",
            "-frames:v",
            "1",
            str(tiny),
        ],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    storage_url = f"{seaweedfs_url}/test-job/tiny.mp4"
    with open(tiny, "rb") as f:
        requests.put(
            storage_url, data=f, headers={"Content-Type": "application/octet-stream"}
        ).raise_for_status()
    return storage_url


@pytest.fixture
def service_patches(mock_nats: tuple[MagicMock, MagicMock]) -> Any:
    mock_nc, mock_js = mock_nats
    with (
        patch("src.service.check_storage_health"),
        patch("src.service.start_health_server"),
        patch("src.service.nats_connect", return_value=(mock_nc, mock_js)),
    ):
        yield mock_nc, mock_js
