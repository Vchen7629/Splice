from types import SimpleNamespace
from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch

import nats.js.errors as js_errors
import pytest

from shared_handler import run_service


def make_settings(**overrides: Any) -> SimpleNamespace:
    defaults = dict(
        SERVICE_NAME="test-service",
        HTTP_PORT=9999,
        SUB_SUBJECT="jobs.video.test",
        PUB_SUBJECT="jobs.complete",
        SUB_QUEUE_NAME="test-workers",
    )
    defaults.update(overrides)
    return SimpleNamespace(**defaults)


async def noop_process_msg(*_args: Any, **_kwargs: Any) -> None:
    pass


@pytest.mark.asyncio
async def test_run_service_calls_consumer(service_patches: Any) -> None:
    with patch(
        "shared_handler.service.consumer", new_callable=AsyncMock
    ) as mock_consumer:
        await run_service(make_settings(), "test-processed", noop_process_msg)

    mock_consumer.assert_called_once()


@pytest.mark.asyncio
async def test_run_service_drains_nats_on_exit(service_patches: Any) -> None:
    mock_nc, _ = service_patches

    with patch("shared_handler.service.consumer", new_callable=AsyncMock):
        await run_service(make_settings(), "test-processed", noop_process_msg)

    mock_nc.drain.assert_called_once()


@pytest.mark.asyncio
async def test_run_service_shuts_down_health_server_on_exit(
    service_patches: Any,
) -> None:
    with (
        patch("shared_handler.service.start_health_server") as mock_health,
        patch("shared_handler.service.consumer", new_callable=AsyncMock),
    ):
        mock_server = MagicMock()
        mock_health.return_value = mock_server
        await run_service(make_settings(), "test-processed", noop_process_msg)

    mock_server.shutdown.assert_called_once()


@pytest.mark.asyncio
async def test_health_server_shutdown_called_even_if_consumer_raises(
    service_patches: Any,
) -> None:
    with (
        patch("shared_handler.service.start_health_server") as mock_health,
        patch(
            "shared_handler.service.consumer",
            new_callable=AsyncMock,
            side_effect=RuntimeError("boom"),
        ),
    ):
        mock_server = MagicMock()
        mock_health.return_value = mock_server
        with pytest.raises(RuntimeError):
            await run_service(make_settings(), "test-processed", noop_process_msg)

    mock_server.shutdown.assert_called_once()


@pytest.mark.asyncio
async def test_drain_called_even_if_consumer_raises(service_patches: Any) -> None:
    mock_nc, _ = service_patches

    with patch(
        "shared_handler.service.consumer",
        new_callable=AsyncMock,
        side_effect=RuntimeError("boom"),
    ):
        with pytest.raises(RuntimeError):
            await run_service(make_settings(), "test-processed", noop_process_msg)

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
            "job-milestones KV bucket not found",
        ),
        (
            lambda js: setattr(
                js, "create_key_value", AsyncMock(side_effect=js_errors.APIError())
            ),
            "failed to create test-processed KV bucket",
        ),
    ],
    ids=["stream_not_found", "job_status_kv_not_found", "kv_creation_fails"],
)
async def test_raises_on_startup_failure(
    service_patches: Any, setup_js: Any, match: str | None
) -> None:
    mock_nc, mock_js = service_patches
    setup_js(mock_js)

    with patch("shared_handler.service.start_health_server") as mock_health:
        mock_server = MagicMock()
        mock_health.return_value = mock_server

        with pytest.raises(RuntimeError, match=match):
            await run_service(make_settings(), "test-processed", noop_process_msg)

    mock_server.shutdown.assert_called_once()
    mock_nc.drain.assert_called_once()


@pytest.mark.asyncio
async def test_consumer_not_called_when_stream_not_found(service_patches: Any) -> None:
    _, mock_js = service_patches
    mock_js.find_stream_name_by_subject = AsyncMock(side_effect=js_errors.NotFoundError)

    with patch(
        "shared_handler.service.consumer", new_callable=AsyncMock
    ) as mock_consumer:
        with pytest.raises(RuntimeError):
            await run_service(make_settings(), "test-processed", noop_process_msg)

    mock_consumer.assert_not_called()
