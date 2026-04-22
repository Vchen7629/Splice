from typing import Any
from unittest.mock import patch
from unittest.mock import AsyncMock
from unittest.mock import MagicMock
import nats.js.errors as js_errors
from src.service import start_service
import pytest


@pytest.mark.asyncio
async def test_start_service_calls_consumer(service_patches: Any) -> None:
    mock_nc, mock_js = service_patches

    with patch("src.service.consumer", new_callable=AsyncMock) as mock_consumer:
        await start_service()

    mock_consumer.assert_called_once()


@pytest.mark.asyncio
async def test_start_service_drains_nats_on_exit(service_patches: Any) -> None:
    mock_nc, mock_js = service_patches

    with patch("src.service.consumer", new_callable=AsyncMock):
        await start_service()

    mock_nc.drain.assert_called_once()


@pytest.mark.asyncio
async def test_start_service_shuts_down_health_server_on_exit(
    service_patches: Any,
) -> None:
    mock_nc, _ = service_patches

    with (
        patch("src.service.start_health_server") as mock_health,
        patch("src.service.consumer", new_callable=AsyncMock),
    ):
        mock_server = MagicMock()
        mock_health.return_value = mock_server
        await start_service()

    mock_server.shutdown.assert_called_once()


@pytest.mark.asyncio
async def test_health_server_shutdown_called_even_if_consumer_raises(
    service_patches: Any,
) -> None:
    with (
        patch("src.service.start_health_server") as mock_health,
        patch(
            "src.service.consumer",
            new_callable=AsyncMock,
            side_effect=RuntimeError("boom"),
        ),
    ):
        mock_server = MagicMock()
        mock_health.return_value = mock_server
        with pytest.raises(RuntimeError):
            await start_service()

    mock_server.shutdown.assert_called_once()


@pytest.mark.asyncio
async def test_drain_called_even_if_consumer_raises(service_patches: Any) -> None:
    mock_nc, _ = service_patches

    with patch(
        "src.service.consumer", new_callable=AsyncMock, side_effect=RuntimeError("boom")
    ):
        with pytest.raises(RuntimeError):
            await start_service()

    mock_nc.drain.assert_called_once()


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
                js, "key_value", AsyncMock(side_effect=js_errors.NotFoundError)
            ),
            "job-status KV bucket not found",
        ),
        (
            lambda js: setattr(
                js, "create_key_value", AsyncMock(side_effect=js_errors.APIError())
            ),
            "failed to create upscale-processed KV bucket",
        ),
    ],
    ids=["stream_not_found", "job_status_kv_not_found", "kv_creation_fails"],
)
async def test_raises_on_startup_failure(
    service_patches: Any, setup_js: Any, match: str | None
) -> None:
    _, mock_js = service_patches
    setup_js(mock_js)

    with pytest.raises(RuntimeError, match=match):
        await start_service()


@pytest.mark.asyncio
async def test_consumer_not_called_when_stream_not_found(service_patches: Any) -> None:
    _, mock_js = service_patches
    mock_js.find_stream_name_by_subject = AsyncMock(side_effect=js_errors.NotFoundError)

    with patch("src.service.consumer", new_callable=AsyncMock) as mock_consumer:
        with pytest.raises(RuntimeError):
            await start_service()

    mock_consumer.assert_not_called()
