from nats import NATS
from nats.js.client import JetStreamContext
from src.nats.client import connect
import pytest

@pytest.mark.asyncio
async def test_connect_returns_connected_clients(nats_url, monkeypatch):
    monkeypatch.setattr("src.nats.client.settings.NATS_URL", nats_url)

    nc, js = await connect()

    assert isinstance(nc, NATS)
    assert isinstance(js, JetStreamContext)
    assert nc.is_connected
    await nc.close()