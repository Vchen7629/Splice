from unittest.mock import patch, MagicMock
from src.storage.check_health import check_storage_health
import requests
import pytest


def test_check_health_succeeds(seaweedfs_url: str, monkeypatch: pytest.MonkeyPatch) -> None:
    """Passes without raising when SeaweedFS master and filer are reachable"""
    monkeypatch.setattr("src.storage.check_health.settings.BASE_STORAGE_URL", seaweedfs_url)
    check_storage_health()


@pytest.mark.parametrize("bad_url", [
    "http://localhost:1",
    "http://doesnotexist.invalid",
])
def test_check_health_raises_on_connection_error(
    bad_url: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable"""
    monkeypatch.setattr("src.storage.check_health.settings.BASE_STORAGE_URL", bad_url)
    with pytest.raises(requests.ConnectionError):
        check_storage_health()


@pytest.mark.parametrize("status_code", [500, 502, 503])
def test_check_health_raises_on_server_error(status_code: int) -> None:
    """Raises HTTPError when SeaweedFS returns a 5xx response"""
    mock_response = MagicMock()
    mock_response.status_code = status_code
    mock_response.raise_for_status.side_effect = requests.HTTPError(
        response=mock_response
    )

    with (
        patch("src.storage.check_health.requests.get", return_value=mock_response),
        pytest.raises(requests.HTTPError),
    ):
        check_storage_health()
