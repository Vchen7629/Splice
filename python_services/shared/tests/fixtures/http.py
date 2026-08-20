from typing import Any
from shared_handler.http import start_health_server
import pytest

@pytest.fixture
def live_http_server() -> Any:
    server = start_health_server(0)
    port = server.server_address[1]
    yield f"http://localhost:{port}"
    server.shutdown()
