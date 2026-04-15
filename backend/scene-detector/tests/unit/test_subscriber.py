from typing import Any, AsyncGenerator
from unittest.mock import patch, MagicMock, AsyncMock
from nats.js.errors import APIError, KeyNotFoundError
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from src.handler.subscriber import raw_videos
from src.handler.messages import SceneSplitMessage, VideoChunkMessage
import json
import pytest


# ── helpers ──────────────────────────────────────────────────────────────────


def make_mock_msg(data: dict[str, Any]) -> AsyncMock:
    msg = AsyncMock()
    msg.data = json.dumps(data).encode()
    return msg


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


def make_mock_js(*msgs: AsyncMock) -> AsyncMock:
    js = AsyncMock(spec=JetStreamContext)
    sub = MagicMock()
    sub.messages = async_iter(list(msgs))
    js.subscribe.return_value = sub
    return js


# ── fixtures ─────────────────────────────────────────────────────────────────


@pytest.fixture
def msg() -> AsyncMock:
    return make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )


@pytest.mark.asyncio
async def test_acks_on_success(mock_kv: AsyncMock, msg: AsyncMock) -> None:
    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(make_mock_js(msg), mock_kv, AsyncMock(spec=KeyValue))

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "process_job_kwargs,publish_kwargs",
    [
        ({"side_effect": Exception("process failed")}, {}),
        ({"return_value": []}, {"side_effect": Exception("publish failed")}),
    ],
    ids=["process_job_fails", "publish_fails"],
)
async def test_naks_on_failure(
    mock_kv: AsyncMock,
    msg: AsyncMock,
    process_job_kwargs: dict[str, Any],
    publish_kwargs: dict[str, Any],
) -> None:
    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            **process_job_kwargs,
        ),
        patch(
            "src.handler.subscriber.scene_video_chunks",
            new_callable=AsyncMock,
            **publish_kwargs,
        ),
    ):
        await raw_videos(make_mock_js(msg), mock_kv, AsyncMock(spec=KeyValue))

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_raises_when_subscribe_fails(mock_kv: AsyncMock) -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.side_effect = APIError()

    with pytest.raises(APIError):
        await raw_videos(mock_js, mock_kv, AsyncMock(spec=KeyValue))


@pytest.mark.asyncio
async def test_acks_and_skips_when_job_already_processed(msg: AsyncMock) -> None:
    already_processed_kv = AsyncMock(spec=KeyValue)
    already_processed_kv.get.return_value = MagicMock()
    mock_js = make_mock_js(msg)

    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ) as mock_process,
        patch("src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, already_processed_kv, AsyncMock(spec=KeyValue))

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    mock_process.assert_not_called()


# ── message routing ───────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_calls_process_job_per_message(mock_kv: AsyncMock) -> None:
    msgs = [
        make_mock_msg(
            {"job_id": "1", "storage_url": "/fake/a.mp4", "target_resolution": "480p"}
        ),
        make_mock_msg(
            {"job_id": "2", "storage_url": "/fake/b.mp4", "target_resolution": "480p"}
        ),
    ]
    mock_js = make_mock_js(*msgs)

    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ) as mock_process,
        patch("src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv, AsyncMock(spec=KeyValue))

    assert mock_process.call_count == 2
    assert mock_process.call_args_list[0][0][0] == SceneSplitMessage(
        job_id="1", storage_url="/fake/a.mp4", target_resolution="480p"
    )
    assert mock_process.call_args_list[1][0][0] == SceneSplitMessage(
        job_id="2", storage_url="/fake/b.mp4", target_resolution="480p"
    )


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
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_js = make_mock_js(msg)

    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=chunk_messages,
        ),
        patch(
            "src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock
        ) as mock_publish,
    ):
        await raw_videos(mock_js, mock_kv, AsyncMock(spec=KeyValue))

    mock_publish.assert_called_once_with(mock_js, chunk_messages)


@pytest.mark.asyncio
async def test_writes_to_kv_on_success() -> None:
    msg = make_mock_msg(
        {
            "job_id": "abc-123",
            "storage_url": "/fake/idk.mp4",
            "target_resolution": "480p",
        }
    )
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.side_effect = KeyNotFoundError()
    mock_js = make_mock_js(msg)

    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv, AsyncMock(spec=KeyValue))

    mock_kv.put.assert_called_once_with("abc-123", b"done")


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "process_job_kwargs,publish_kwargs",
    [
        ({"side_effect": Exception("process failed")}, {}),
        ({"return_value": []}, {"side_effect": Exception("publish failed")}),
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
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            **process_job_kwargs,
        ),
        patch(
            "src.handler.subscriber.scene_video_chunks",
            new_callable=AsyncMock,
            **publish_kwargs,
        ),
    ):
        await raw_videos(make_mock_js(msg), mock_kv, AsyncMock(spec=KeyValue))

    mock_kv.put.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_status_error_logs_and_continues(
    mock_kv: AsyncMock, msg: AsyncMock
) -> None:
    """When job_status_kv.put raises, the error is logged and message is still acked"""
    mock_job_status_kv = AsyncMock(spec=KeyValue)
    mock_job_status_kv.put.side_effect = Exception("kv write failed")

    with (
        patch(
            "src.handler.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=[],
        ),
        patch("src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(make_mock_js(msg), mock_kv, mock_job_status_kv)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_stage_written_to_job_status_kv_before_processing(
    mock_kv: AsyncMock,
) -> None:
    """job_status_kv.put is called with PROCESSING:scene-detector before process_job runs"""
    msg = make_mock_msg(
        {
            "job_id": "abc-123",
            "storage_url": "/fake/idk.mp4",
            "target_resolution": "480p",
        }
    )
    mock_js = make_mock_js(msg)
    mock_job_status_kv = AsyncMock(spec=KeyValue)
    call_order: list[str] = []

    async def fake_process_job(_metadata: Any) -> list[Any]:
        call_order.append("process_job")
        return []

    async def fake_job_status_put(key: str, value: bytes) -> None:
        call_order.append("job_status_put")

    mock_job_status_kv.put.side_effect = fake_job_status_put

    with (
        patch("src.handler.subscriber.process_job", side_effect=fake_process_job),
        patch("src.handler.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv, mock_job_status_kv)

    expected_payload = json.dumps(
        {"state": "PROCESSING", "stage": "scene-detector"}
    ).encode()
    mock_job_status_kv.put.assert_called_once_with("abc-123", expected_payload)
    assert call_order == ["job_status_put", "process_job"]
