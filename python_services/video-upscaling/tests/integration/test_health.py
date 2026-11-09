import json
import urllib.error
import urllib.request
from typing import Generator

import pytest

from health import start_health_server


@pytest.fixture
def live_http_server() -> Generator[str, None, None]:
    server = start_health_server(0)
    port = server.server_address[1]
    yield f"http://localhost:{port}"
    server.shutdown()


def test_health_endpoint_returns_200(live_http_server: str) -> None:
    with urllib.request.urlopen(f"{live_http_server}/health") as resp:
        assert resp.status == 200
        assert json.loads(resp.read()) == {"status": "Healthy"}


def test_unknown_path_returns_404(live_http_server: str) -> None:
    with pytest.raises(urllib.error.HTTPError) as exc_info:
        urllib.request.urlopen(f"{live_http_server}/not-found")
    assert exc_info.value.code == 404
