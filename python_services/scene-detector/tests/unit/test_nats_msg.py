from typing import Any
from unittest.mock import patch
from unittest.mock import MagicMock
from unittest.mock import AsyncMock
from nats.aio.client import Client as NATSClient
from nats.js.kv import KeyValue
from nats.js.errors import KeyNotFoundError
from nats.js.client import JetStreamContext
from shared_handler import VideoChunkMessage
from src.processing.nats_msg import process_msg
from src.core.settings import settings
from test_helpers.nats import milestone_entry
import json
import pytest

MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_JS = AsyncMock(spec=JetStreamContext)
MOCK_KV = AsyncMock(spec=KeyValue)
MOCK_KV.get.return_value = milestone_entry("PROCESSING", "upload")


def make_mock_msg(data: dict[str, Any]) -> AsyncMock:
    msg = AsyncMock()
    msg.data = json.dumps(data).encode()
    return msg


@pytest.fixture
def msg() -> AsyncMock:
    return make_mock_msg(
        {
            "job_id": "1",
            "storage_url": "/fake/idk.mp4",
            "source_resolution": "1080p",
            "target_resolution": "480p",
        }
    )


@pytest.mark.asyncio
async def test_acks_on_success(mock_kv: AsyncMock, msg: AsyncMock) -> None:
    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, MOCK_KV, msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


_one_chunk = [
    VideoChunkMessage(
        job_id="1",
        chunk_index=0,
        total_chunks=1,
        storage_url="/tmp/c.mp4",
        target_resolution="480p",
    )
]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "process_job_kwargs,publish_kwargs",
    [
        ({"side_effect": Exception("process failed")}, {}),
        ({"return_value": _one_chunk}, {"side_effect": Exception("publish failed")}),
    ],
    ids=["process_job_fails", "publish_fails"],
)
async def test_updates_kv_and_acks_on_failure(
    mock_kv: AsyncMock,
    msg: AsyncMock,
    process_job_kwargs: dict[str, Any],
    publish_kwargs: dict[str, Any],
) -> None:
    job_status_kv = AsyncMock(spec=KeyValue)
    job_status_kv.get.return_value = milestone_entry("PROCESSING", "upload")

    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            **process_job_kwargs,
        ),
        patch(
            "src.processing.nats_msg.publisher",
            new_callable=AsyncMock,
            **publish_kwargs,
        ),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, job_status_kv, msg)

    job_id, payload = job_status_kv.update.call_args_list[-1][0]
    assert job_id == "1"
    assert json.loads(payload)["state"] == "FAILED"
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_acks_and_skips_when_job_already_processed(msg: AsyncMock) -> None:
    already_processed_kv = AsyncMock(spec=KeyValue)
    already_processed_kv.get.return_value = MagicMock()

    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ) as mock_process,
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock),
    ):
        await process_msg(MOCK_NC, MOCK_JS, already_processed_kv, MOCK_KV, msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    mock_process.assert_not_called()


@pytest.mark.asyncio
async def test_passes_chunk_messages_to_publisher(mock_kv: AsyncMock) -> None:
    chunk_messages = [
        VideoChunkMessage(
            job_id="1",
            chunk_index=0,
            total_chunks=1,
            storage_url="/tmp/chunk-001.mp4",
            target_resolution="480p",
        )
    ]
    msg = make_mock_msg(
        {
            "job_id": "1",
            "storage_url": "/fake/idk.mp4",
            "source_resolution": "1080p",
            "target_resolution": "480p",
        }
    )
    mock_js = AsyncMock(spec=JetStreamContext)

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
        await process_msg(MOCK_NC, mock_js, mock_kv, MOCK_KV, msg)

    mock_publish.assert_called_once_with(
        mock_js, chunk_messages[0], settings.PUB_SUBJECT, settings.SERVICE_NAME
    )


@pytest.mark.asyncio
async def test_writes_to_kv_on_success() -> None:
    msg = make_mock_msg(
        {
            "job_id": "abc-123",
            "storage_url": "/fake/idk.mp4",
            "source_resolution": "1080p",
            "target_resolution": "480p",
        }
    )
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.side_effect = KeyNotFoundError()

    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, MOCK_KV, msg)

    mock_kv.put.assert_called_once_with("abc-123", b"done")


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "process_job_kwargs,publish_kwargs",
    [
        ({"side_effect": Exception("process failed")}, {}),
        ({"return_value": _one_chunk}, {"side_effect": Exception("publish failed")}),
    ],
    ids=["process_job_fails", "publish_fails"],
)
async def test_does_not_write_to_kv_on_failure(
    mock_kv: AsyncMock,
    msg: AsyncMock,
    process_job_kwargs: dict[str, Any],
    publish_kwargs: dict[str, Any],
) -> None:
    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            **process_job_kwargs,
        ),
        patch(
            "src.processing.nats_msg.publisher",
            new_callable=AsyncMock,
            **publish_kwargs,
        ),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, MOCK_KV, msg)

    mock_kv.put.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_error_records_failure_and_acks(
    mock_kv: AsyncMock, msg: AsyncMock
) -> None:
    """When job_milestone_kv.put fails for the stage write but succeeds for the
    failed-write fallback, the failure is durably recorded and the message is acked."""
    mock_job_milestone_kv = AsyncMock(spec=KeyValue)
    mock_job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    mock_job_milestone_kv.update.side_effect = [Exception("kv write failed"), None]

    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, mock_job_milestone_kv, msg)

    assert mock_job_milestone_kv.update.call_count == 2
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_error_naks_when_failure_write_also_fails(
    mock_kv: AsyncMock, msg: AsyncMock
) -> None:
    """When job_milestone_kv.put fails for both the stage write and the failed-write
    fallback, nothing was durably recorded, so the message is nak'd for redelivery."""
    mock_job_milestone_kv = AsyncMock(spec=KeyValue)
    mock_job_milestone_kv.put.side_effect = Exception("kv write failed")

    with (
        patch(
            "src.processing.nats_msg.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, mock_job_milestone_kv, msg)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_stage_written_to_job_status_kv_before_processing(
    mock_kv: AsyncMock,
) -> None:
    """job_status_kv.update is called with PROCESSING:scene-detector before process_job runs"""
    msg = make_mock_msg(
        {
            "job_id": "abc-123",
            "storage_url": "/fake/idk.mp4",
            "source_resolution": "1080p",
            "target_resolution": "480p",
        }
    )
    mock_job_status_kv = AsyncMock(spec=KeyValue)
    mock_job_status_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    call_order: list[str] = []

    async def fake_process_job(_metadata: Any, _on_progress: Any = None) -> list[Any]:
        call_order.append("process_job")
        return []

    async def fake_job_status_update(key: str, value: bytes, last: int) -> None:
        call_order.append("job_status_update")

    mock_job_status_kv.update.side_effect = fake_job_status_update

    with (
        patch("src.processing.nats_msg.process_job", side_effect=fake_process_job),
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock),
    ):
        await process_msg(MOCK_NC, MOCK_JS, mock_kv, mock_job_status_kv, msg)

    expected_payload = json.dumps(
        {"state": "PROCESSING", "stage": "scene-detector"}
    ).encode()
    mock_job_status_kv.update.assert_called_once_with(
        "abc-123", expected_payload, last=1
    )
    assert call_order == ["job_status_update", "process_job"]
