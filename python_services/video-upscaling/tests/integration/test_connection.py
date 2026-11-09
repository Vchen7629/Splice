from unittest.mock import MagicMock

import pytest
from nats.aio.client import Client as NATSClient
from nats.js.client import JetStreamContext

from core import settings
from msgbus import nats_connect


@pytest.mark.asyncio
async def test_nats_connect_returns_connected_clients(
    nats_url: str, mock_logger: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(settings, "NATS_URL", nats_url)

    nc, js = await nats_connect(mock_logger)

    assert isinstance(nc, NATSClient)
    assert isinstance(js, JetStreamContext)
    assert nc.is_connected
    await nc.close()
