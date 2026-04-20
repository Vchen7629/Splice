import json
import urllib.request
import urllib.error
import pytest


def test_health_endpoint_returns_200(live_http_server: str) -> None:
    with urllib.request.urlopen(f"{live_http_server}/health") as resp:
        assert resp.status == 200
        assert json.loads(resp.read()) == {"status": "Healthy"}


def test_unknown_path_returns_404(live_http_server: str) -> None:
    with pytest.raises(urllib.error.HTTPError) as exc_info:
        urllib.request.urlopen(f"{live_http_server}/not-found")
    assert exc_info.value.code == 404
