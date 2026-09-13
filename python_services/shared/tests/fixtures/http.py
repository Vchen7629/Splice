from typing import Any

import pytest

from shared_handler import start_health_server


@pytest.fixture
def live_http_server() -> Any:
    server = start_health_server(0)
    port = server.server_address[1]
    yield f"http://localhost:{port}"
    server.shutdown()
