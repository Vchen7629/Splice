from pathlib import Path
from shared_storage import queries
import pytest


@pytest.fixture(autouse=True)
def patch_temp_dir(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Redirect fetch_video writes to pytest's tmp_path so cleanup is automatic"""
    monkeypatch.setattr(queries, "TEMP_DIR", str(tmp_path))
