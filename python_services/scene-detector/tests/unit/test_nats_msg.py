from typing import Any
from unittest.mock import AsyncMock, patch

import pytest
from nats.aio.client import Client as NATSClient
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from shared_handler import ProcessJobMessage, ProcessJobMsgContext, VideoChunkMessage

from src.core.settings import settings
from src.processing.nats_msg import process_job_msg


def make_ctx(**overrides: Any) -> ProcessJobMsgContext:
    MOCK_KV = AsyncMock(spec=KeyValue)

    metadata = overrides.pop(
        "metadata",
        ProcessJobMessage(
            job_id="1",
            storage_url="/fake/idk.mp4",
            source_resolution="1080p",
            target_resolution="480p",
        ),
    )
    defaults: dict[str, Any] = dict(
        nc=AsyncMock(spec=NATSClient),
        js=AsyncMock(spec=JetStreamContext),
        msg_processed_kv=MOCK_KV,
        job_milestone_kv=MOCK_KV,
    )
    defaults.update(overrides)
    return ProcessJobMsgContext(metadata=metadata, **defaults)


@pytest.mark.asyncio
async def test_passes_chunk_messages_to_publisher() -> None:
    chunk_messages = [
        VideoChunkMessage(
            job_id="1",
            chunk_index=0,
            total_chunks=1,
            storage_url="/tmp/chunk-001.mp4",
            target_resolution="480p",
        )
    ]
    mock_js = AsyncMock(spec=JetStreamContext)
    ctx = make_ctx(js=mock_js)

    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            return_value=chunk_messages,
        ),
        patch(
            "src.processing.nats_msg.publisher", new_callable=AsyncMock
        ) as mock_publish,
    ):
        await process_job_msg(ctx, AsyncMock())

    mock_publish.assert_called_once_with(
        mock_js, chunk_messages[0], settings.PUB_SUBJECT, settings.SERVICE_NAME
    )
