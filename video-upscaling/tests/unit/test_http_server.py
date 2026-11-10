import json
import threading
from http.server import ThreadingHTTPServer
from typing import Any
from unittest.mock import MagicMock, create_autospec, patch

import pytest
from prometheus_client.parser import text_string_to_metric_families

from http_server import HTTPHandler, start_http_server


def make_handler(path: str) -> MagicMock:
    handler = create_autospec(HTTPHandler, instance=True)
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
def test_health_and_none_endpoint(
    path: str, expected_status: int, expected_body: dict[str, Any] | None
) -> None:
    handler = make_handler(path)
    HTTPHandler.do_GET(handler)

    handler.send_response.assert_called_once_with(expected_status)
    handler.end_headers.assert_called_once()
    if expected_body is not None:
        handler.send_header.assert_called_once_with("Content-Type", "application/json")
        assert json.loads(handler.wfile.write.call_args[0][0]) == expected_body
    else:
        handler.wfile.write.assert_not_called()


def test_metrics_endpoint() -> None:
    handler = make_handler("/metrics")
    HTTPHandler.do_GET(handler)

    handler.send_response.assert_called_once_with(200)
    handler.end_headers.assert_called_once()

    body = handler.wfile.write.call_args[0][0].decode()
    families = list(text_string_to_metric_families(body))
    assert families  # parses as valid exposition format


def test_start_http_server() -> None:
    mock_server = MagicMock(spec=ThreadingHTTPServer)
    real_thread_cls = threading.Thread
    created_threads: list[MagicMock] = []
    captured_kwargs: list[dict[str, Any]] = []

    def capture_thread(**kwargs: object) -> MagicMock:
        captured_kwargs.append(kwargs)  # type: ignore[arg-type]
        t = MagicMock(spec=real_thread_cls)
        created_threads.append(t)
        return t

    with (
        patch("http_server.ThreadingHTTPServer", return_value=mock_server),
        patch("http_server.threading.Thread", side_effect=capture_thread),
    ):
        result = start_http_server(9099)

    assert result is mock_server
    assert captured_kwargs[0].get("daemon") is True
    created_threads[0].start.assert_called_once()
