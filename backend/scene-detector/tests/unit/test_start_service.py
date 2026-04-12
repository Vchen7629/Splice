from unittest.mock import patch
from unittest.mock import MagicMock
from unittest.mock import AsyncMock
from src.service import start_service
import pytest
import nats.js.errors as js_errors


@pytest.mark.asyncio
async def test_raises_on_runtime_error() -> None:
    """It should raise the RuntimeError when stream is not found"""
    mock_js = MagicMock()
    mock_js.find_stream_name_by_subject = AsyncMock(side_effect=js_errors.NotFoundError)

    mock_nc = MagicMock()
    mock_nc.drain = AsyncMock()

    with (
        patch("src.service.check_storage_health"),
        patch("src.service.nats_connect", return_value=(mock_nc, mock_js)),
        pytest.raises(RuntimeError),
    ):
        await start_service()


@pytest.mark.asyncio
async def test_raises_runtime_error_when_kv_creation_fails() -> None:
    """It should raise RuntimeError when the KV bucket cannot be created"""
    mock_js = MagicMock()
    mock_js.find_stream_name_by_subject = AsyncMock()
    mock_js.create_key_value = AsyncMock(side_effect=js_errors.APIError())

    mock_nc = MagicMock()
    mock_nc.drain = AsyncMock()

    with (
        patch("src.service.check_storage_health"),
        patch("src.service.nats_connect", return_value=(mock_nc, mock_js)),
        pytest.raises(
            RuntimeError, match="failed to create scene-split-processed KV bucket"
        ),
    ):
        await start_service()
