from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from nats.aio.client import Client as NATSClient
from nats.errors import AuthorizationError, NoServersError, TimeoutError
from nats.js.client import JetStreamContext

from msgbus.connection import nats_connect


@pytest.mark.asyncio
@pytest.mark.parametrize(
    argnames="exc", argvalues=[NoServersError(), AuthorizationError(), TimeoutError()]
)
async def test_nats_connect_raises_on_nats_failure(
    exc: Any, mock_logger: MagicMock
) -> None:
    """It should raise the error when caught"""
    with patch("msgbus.connection.NATSClient") as mock_client_class:
        mock_instance = MagicMock(spec=NATSClient)
        mock_instance.connect = AsyncMock(side_effect=exc)
        mock_client_class.return_value = mock_instance
        with pytest.raises(type(exc)):
            await nats_connect(mock_logger)


@pytest.mark.asyncio
async def test_nats_connect_returns_nats_and_jetstream(mock_logger: MagicMock) -> None:
    mock_js = MagicMock(spec=JetStreamContext)
    mock_ns = MagicMock(spec=NATSClient)
    mock_ns.connect = AsyncMock()
    mock_ns.jetstream.return_value = mock_js

    with patch("msgbus.connection.NATSClient", return_value=mock_ns):
        nc, js = await nats_connect(mock_logger)

    assert nc is mock_ns
    assert js is mock_js
