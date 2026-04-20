from typing import Any
from socket import socket
from shared_handler.http_server import start_health_server
import pytest


def _free_port() -> int:
    with socket() as s:
        s.bind(("", 0))
        return s.getsockname()[1]


@pytest.fixture
def live_http_server() -> Any:
    port = _free_port()
    server = start_health_server(port)
    yield f"http://localhost:{port}"
    server.shutdown()
