from typing import Any
from unittest.mock import MagicMock, patch

import pytest


@pytest.fixture
def service_patches(mock_nats: tuple[MagicMock, MagicMock]) -> Any:
    """Patches check_storage_health, start_health_server, and nats_connect with mocked nats"""
    mock_nc, mock_js = mock_nats
    with (
        patch("shared_handler.service.check_storage_health"),
        patch("shared_handler.service.start_health_server"),
        patch("shared_handler.service.nats_connect", return_value=(mock_nc, mock_js)),
    ):
        yield mock_nc, mock_js
