from typing import Any
from typing import AsyncGenerator
from pathlib import Path
from nats.js import JetStreamContext
from unittest.mock import patch
from src.handler.http_server import start_health_server
from src.storage import queries
import socket
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
def chunk_files(tmp_path: Path) -> list[str]:
    """Creates a set of fake .mp4 chunk files in tmp_path"""
    chunks = []
    for i in range(3):
        chunk = tmp_path / f"video-Scene-{i + 1:03d}.mp4"
        chunk.write_bytes(b"fake chunk content")
        chunks.append(str(chunk))
    return chunks


@pytest.fixture
def single_video_chunk(tmp_path: Path) -> str:
    chunk = tmp_path / "chunk.mp4"
    chunk.write_bytes(b"data")
    return str(chunk)


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("", 0))
        return s.getsockname()[1]


@pytest.fixture
def live_http_server() -> Any:
    port = _free_port()
    server = start_health_server(port)
    yield f"http://localhost:{port}"
    server.shutdown()


@pytest.fixture
def spy_drain(js_context: tuple[Any, JetStreamContext]) -> tuple[Any, list[bool]]:
    """Replaces nc.drain with a no-op spy (whatever that means)"""
    nc, _ = js_context
    called: list[bool] = []

    async def _spy() -> None:
        called.append(True)

    nc.drain = _spy
    return nc, called
