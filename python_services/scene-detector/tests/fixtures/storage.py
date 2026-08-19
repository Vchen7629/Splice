from typing import Generator
from testcontainers.core.container import DockerContainer
import requests
import pytest
import time
import os

TEST_VIDEO_PATH = os.path.join(
    os.path.dirname(__file__), "..", "videos", "ForBiggerBlazes.mp4"
)
TEST_VIDEO_FILENAME = "ForBiggerBlazes.mp4"


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
