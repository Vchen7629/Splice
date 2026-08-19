from typing import Any
from unittest.mock import patch, MagicMock, AsyncMock
from src.service import start_service
import pytest
import nats.js.errors as js_errors


@pytest.fixture
def service_patches(mock_nats: tuple[MagicMock, MagicMock]) -> Any:
    mock_nc, mock_js = mock_nats
    with (
        patch("src.service.check_storage_health"),
        patch("src.service.start_health_server"),
        patch("src.service.nats_connect", return_value=(mock_nc, mock_js)),
    ):
        yield mock_nc, mock_js


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "setup_js,match",
    [
        (
            lambda js: setattr(
                js,
                "find_stream_name_by_subject",
                AsyncMock(side_effect=js_errors.NotFoundError),
            ),
            None,
        ),
        (
            lambda js: setattr(
                js, "create_key_value", AsyncMock(side_effect=js_errors.APIError())
            ),
            "failed to create scene-split-processed KV bucket",
        ),
        (
            lambda js: setattr(
                js, "key_value", AsyncMock(side_effect=js_errors.NotFoundError)
            ),
            "job-status KV bucket not found",
        ),
    ],
    ids=["stream_not_found", "kv_creation_fails", "job_status_kv_not_found"],
)
async def test_raises_runtime_error(
    service_patches: Any,
    setup_js: Any,
    match: str | None,
) -> None:
    _, mock_js = service_patches
    setup_js(mock_js)
    with pytest.raises(RuntimeError, match=match):
        await start_service()
