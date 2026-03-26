from nats import NATS
from nats.js.client import JetStreamContext
from src.nats.connection import nats_connect
import pytest

@pytest.mark.asyncio
async def test_connect_returns_connected_clients(nats_url, monkeypatch):
    monkeypatch.setattr("src.nats.connection.settings.NATS_URL", nats_url)

    nc, js = await nats_connect()

    assert isinstance(nc, NATS)
    assert isinstance(js, JetStreamContext)
    assert nc.is_connected
    await nc.close()