import asyncio
from typing import Any

import pytest
from nats.js.client import JetStreamContext

from msgbus import UpscaleCompleteMsg, publisher


@pytest.mark.asyncio
async def test_publishes_all_messages_with_correct_payload(
    js_context: tuple[Any, JetStreamContext],
    complete_subscriber: tuple[list[dict[str, Any]], asyncio.Event],
) -> None:
    _, js = js_context
    received, _ = complete_subscriber

    for job_id in ("1", "2"):
        await publisher(js, UpscaleCompleteMsg(job_id=job_id))

    async def _wait_for_delivery() -> None:
        while len(received) < 2:
            await asyncio.sleep(0.05)

    await asyncio.wait_for(_wait_for_delivery(), timeout=5)

    assert received == [{"job_id": "1"}, {"job_id": "2"}]
