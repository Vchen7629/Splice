from src.storage import queries
from pathlib import Path
import pytest


@pytest.fixture(autouse=True)
def patch_temp_dir(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Redirect fetch_video writes to pytest's tmp_path so cleanup is automatic"""
    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))


@pytest.fixture
def chunk_files(tmp_path: Path) -> list[str]:
    """Creates a set of fake .mp4 chunk files in tmp_path"""
    chunks = []
    for i in range(3):
        chunk = tmp_path / f"video-Scene-{i + 1:03d}.mp4"
        chunk.write_bytes(b"fake chunk content")
        chunks.append(str(chunk))
    return chunks
