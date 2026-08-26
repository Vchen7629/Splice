from typing import Generator
from unittest.mock import AsyncMock, MagicMock
from testcontainers.nats import NatsContainer
import pytest


@pytest.fixture(scope="session")
def nats_url() -> Generator[str, None, None]:
    """Starts a nats container and returns url"""
    with NatsContainer(jetstream=True) as container:
        yield container.nats_uri()


@pytest.fixture
def mock_nats() -> tuple[MagicMock, MagicMock]:
    mock_js = MagicMock()
    mock_js.find_stream_name_by_subject = AsyncMock()
    mock_js.create_key_value = AsyncMock()
    mock_js.key_value = AsyncMock()
    mock_nc = MagicMock()
    mock_nc.is_closed = False
    mock_nc.drain = AsyncMock()
    return mock_nc, mock_js
