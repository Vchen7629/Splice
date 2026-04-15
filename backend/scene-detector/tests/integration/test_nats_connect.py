from typing import Any
from nats.aio.client import Client as NATSClient
from nats.js.client import JetStreamContext
from src.handler.connection import nats_connect
import pytest


@pytest.mark.asyncio
async def test_connect_returns_connected_clients(
    nats_url: str, monkeypatch: Any
) -> None:
    monkeypatch.setattr("src.handler.connection.settings.NATS_URL", nats_url)

    nc, js = await nats_connect()

    assert isinstance(nc, NATSClient)
    assert isinstance(js, JetStreamContext)
    assert nc.is_connected
    await nc.close()
