from typing import Any
from http.server import HTTPServer
from unittest.mock import MagicMock, create_autospec, patch
from src.handler.http_server import HealthEnpointHandler, start_health_server
import json
import pytest
import threading


def make_handler(path: str) -> MagicMock:
    handler = create_autospec(HealthEnpointHandler, instance=True)
    handler.path = path
    handler.wfile = MagicMock()
    return handler


@pytest.mark.parametrize(
    "path,expected_status,expected_body",
    [
        ("/health", 200, {"status": "Healthy"}),
        ("/unknown", 404, None),
    ],
    ids=["health", "not_found"],
)
def test_endpoint(
    path: str, expected_status: int, expected_body: dict[str, Any] | None
) -> None:
    handler = make_handler(path)
    HealthEnpointHandler.do_GET(handler)

    handler.send_response.assert_called_once_with(expected_status)
    handler.end_headers.assert_called_once()
    if expected_body is not None:
        handler.send_header.assert_called_once_with("Content-Type", "application/json")
        assert json.loads(handler.wfile.write.call_args[0][0]) == expected_body
    else:
        handler.wfile.write.assert_not_called()


# ── server startup ────────────────────────────────────────────────────────────


def test_start_health_server() -> None:
    mock_server = MagicMock(spec=HTTPServer)
    real_thread_cls = threading.Thread
    created_threads: list[MagicMock] = []
    captured_kwargs: list[dict[str, Any]] = []

    def capture_thread(**kwargs: object) -> MagicMock:
        captured_kwargs.append(kwargs)  # type: ignore[arg-type]
        t = MagicMock(spec=real_thread_cls)
        created_threads.append(t)
        return t

    with (
        patch("src.handler.http_server.HTTPServer", return_value=mock_server),
        patch("src.handler.http_server.threading.Thread", side_effect=capture_thread),
    ):
        result = start_health_server(9099)

    assert result is mock_server
    assert captured_kwargs[0].get("daemon") is True
    created_threads[0].start.assert_called_once()
