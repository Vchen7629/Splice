from typing import Generator
from testcontainers.core.container import DockerContainer
import time
import uuid
import requests
import pytest


def _wait_for_seaweedfs(
    host: str, master_port: int, filer_port: int, timeout: int = 60
) -> None:
    """Poll SeaweedFS until master/filer HTTP endpoints respond and filer can accept a write"""
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
            raise TimeoutError(f"SeaweedFS not ready at {url} after {timeout}s")

    # The endpoints above can respond before the volume server has finished
    # registering with the master, which makes the *first* real write 500
    # with "no writable volumes". Probe with an actual write until it
    # succeeds so callers don't race that startup window.
    probe_url = f"http://{host}:{filer_port}/_readiness_probe/{uuid.uuid4()}"
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            resp = requests.put(probe_url, data=b"ready", timeout=2)
            if resp.status_code < 300:
                requests.delete(probe_url, timeout=2)
                return
        except Exception:
            pass
        time.sleep(1)

    raise TimeoutError(f"SeaweedFS filer not accepting writes after {timeout}s")


@pytest.fixture(scope="session")
def seaweedfs_url() -> Generator[str, None, None]:
    """Starts a SeaweedFS container and yields the filer base URL (http://host:8888)"""
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
