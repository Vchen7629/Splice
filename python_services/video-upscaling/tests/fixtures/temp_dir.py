from pathlib import Path

import pytest

from core import settings


@pytest.fixture(autouse=True)
def patch_temp_dir(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Redirect TEMP_ROOT to pytest's tmp_path so job files are cleaned up automatically"""
    monkeypatch.setattr(settings, "TEMP_ROOT", tmp_path)
