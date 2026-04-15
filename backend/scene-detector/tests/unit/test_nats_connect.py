from typing import Any
from unittest.mock import patch
from unittest.mock import AsyncMock
from unittest.mock import MagicMock
from nats.aio.client import Client as NATSClient
from nats.errors import TimeoutError
from nats.errors import NoServersError
from nats.errors import AuthorizationError
from nats.js.client import JetStreamContext
from src.handler.connection import nats_connect
import pytest


@pytest.mark.asyncio
@pytest.mark.parametrize(
    argnames="exc", argvalues=[NoServersError(), AuthorizationError(), TimeoutError()]
)
async def test_connect_raises_on_nats_failure(exc: Any) -> None:
    """It should raise the error when caught"""
    with patch("src.handler.connection.NATSClient") as mock_client_class:
        mock_instance = MagicMock(spec=NATSClient)
        mock_instance.connect = AsyncMock(side_effect=exc)
        mock_client_class.return_value = mock_instance
        with pytest.raises(type(exc)):
            await nats_connect()


@pytest.mark.asyncio
async def test_connect_returns_nats_and_jetstream() -> None:
    mock_js = MagicMock(spec=JetStreamContext)
    mock_ns = MagicMock(spec=NATSClient)
    mock_ns.connect = AsyncMock()
    mock_ns.jetstream.return_value = mock_js

    with patch("src.handler.connection.NATSClient", return_value=mock_ns):
        nc, js = await nats_connect()

    assert nc is mock_ns
    assert js is mock_js
