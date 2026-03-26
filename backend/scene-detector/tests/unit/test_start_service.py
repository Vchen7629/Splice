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

    with patch("src.service.nats_connect", return_value=(mock_nc, mock_js)):
        with pytest.raises(RuntimeError):
            await start_service()
