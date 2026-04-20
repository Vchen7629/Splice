from typing import Any
from typing import AsyncGenerator
from pathlib import Path
from nats.js import JetStreamContext
from unittest.mock import patch
from shared_storage import queries
import pytest
import pytest_asyncio


@pytest.fixture(autouse=True)
def patch_temp_dir(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Redirect fetch_video writes to pytest's tmp_path so cleanup is automatic"""
    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))


@pytest_asyncio.fixture
async def patched_start_service(
    js_context: tuple[Any, JetStreamContext],
) -> AsyncGenerator[tuple[Any, JetStreamContext], None]:
    """Yields (nc, js) with check_storage_health, start_health_server, and nats_connect patched"""
    nc, js = js_context
    with (
        patch("src.service.check_storage_health"),
        patch("src.service.start_health_server"),
        patch("src.service.nats_connect", return_value=(nc, js)),
    ):
        yield nc, js


@pytest.fixture
def spy_drain(js_context: tuple[Any, JetStreamContext]) -> tuple[Any, list[bool]]:
    """Replaces nc.drain with a no-op spy (whatever that means)"""
    nc, _ = js_context
    called: list[bool] = []

    async def _spy() -> None:
        called.append(True)

    nc.drain = _spy
    return nc, called
