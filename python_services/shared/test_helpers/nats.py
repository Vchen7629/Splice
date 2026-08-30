from typing import Generator
from unittest.mock import AsyncMock, MagicMock
from testcontainers.nats import NatsContainer
from shared_handler import ProcessJobMessage
import json
import pytest


@pytest.fixture(scope="session")
def nats_url() -> Generator[str, None, None]:
    """Starts a nats container and returns url"""
    with NatsContainer(jetstream=True) as container:
        yield container.nats_uri()


@pytest.fixture
def mock_nats() -> tuple[MagicMock, MagicMock]:
    mock_js = MagicMock()
    mock_js.find_stream_name_by_subject = AsyncMock()
    mock_js.create_key_value = AsyncMock()
    mock_js.key_value = AsyncMock()
    mock_nc = MagicMock()
    mock_nc.is_closed = False
    mock_nc.drain = AsyncMock()
    return mock_nc, mock_js


def milestone_entry(state: str, stage: str = "", revision: int = 1) -> MagicMock:
    payload = {"state": state, "stage": stage} if stage else {"state": state}

    return MagicMock(value=json.dumps(payload).encode(), revision=revision)


def make_msg(
    job_id: str = "job-123",
    storage_url: str = "http://storage/video.mp4",
    source_resolution: str = "480p",
    target_resolution: str = "1080p",
) -> AsyncMock:
    """Build a mock NATS Msg with a valid ProcessJobMessage payload."""
    payload = ProcessJobMessage(
        job_id=job_id,
        storage_url=storage_url,
        source_resolution=source_resolution,
        target_resolution=target_resolution,
    )
    msg = AsyncMock()
    msg.data = payload.model_dump_json().encode()
    return msg
