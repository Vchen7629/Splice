from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from unittest.mock import AsyncMock
import pytest


@pytest.fixture
def mock_kv() -> AsyncMock:
    kv = AsyncMock(spec=KeyValue)
    kv.get.side_effect = KeyNotFoundError()
    return kv
