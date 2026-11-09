from typing import Any
from unittest.mock import AsyncMock

import pytest
from nats.errors import TimeoutError
from nats.js.client import JetStreamContext
from nats.js.errors import APIError

from msgbus import UpscaleCompleteMsg, publisher


@pytest.mark.asyncio
@pytest.mark.parametrize(argnames="exc", argvalues=[APIError(), TimeoutError()])
async def test_raises_on_publish_failure(exc: Any) -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.publish.side_effect = exc

    with pytest.raises(type(exc)):
        await publisher(mock_js, UpscaleCompleteMsg(job_id="1"))


@pytest.mark.asyncio
async def test_calls_publish_per_msg() -> None:
    """publish should be called once per published message"""
    mock_js = AsyncMock(spec=JetStreamContext)

    for job_id in ("1", "2"):
        await publisher(mock_js, UpscaleCompleteMsg(job_id=job_id))

    assert mock_js.publish.call_count == 2
