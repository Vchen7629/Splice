from typing import Any, AsyncGenerator
from unittest.mock import patch, MagicMock, AsyncMock
from nats.js.errors import APIError, KeyNotFoundError
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from src.nats.subscriber import raw_videos
from src.nats.messages import SceneSplitMessage, VideoChunkMessage
import json
import pytest


def make_mock_msg(data: dict[str, Any]) -> AsyncMock:
    msg = AsyncMock()
    msg.data = json.dumps(data).encode()
    return msg


def make_mock_kv(already_processed: bool = False) -> MagicMock:
    mock_kv = AsyncMock(spec=KeyValue)
    if already_processed:
        mock_kv.get.return_value = MagicMock()
    else:
        mock_kv.get.side_effect = KeyNotFoundError()
    return mock_kv


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


@pytest.mark.asyncio
async def test_acks_on_success() -> None:
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ),
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_naks_when_process_job_fails() -> None:
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job",
            new_callable=AsyncMock,
            side_effect=Exception("failed"),
        ),
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_naks_when_publish_fails() -> None:
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ),
        patch(
            "src.nats.subscriber.scene_video_chunks",
            new_callable=AsyncMock,
            side_effect=Exception("publish failed"),
        ),
    ):
        await raw_videos(mock_js, mock_kv)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_raises_when_subscribe_fails() -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.side_effect = APIError()
    mock_kv = make_mock_kv()

    with pytest.raises(APIError):
        await raw_videos(mock_js, mock_kv)


@pytest.mark.asyncio
async def test_calls_process_job_per_message() -> None:
    msgs = [
        make_mock_msg(
            {"job_id": "1", "storage_url": "/fake/a.mp4", "target_resolution": "480p"}
        ),
        make_mock_msg(
            {"job_id": "2", "storage_url": "/fake/b.mp4", "target_resolution": "480p"}
        ),
    ]
    mock_sub = MagicMock()
    mock_sub.messages = async_iter(msgs)
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ) as mock_process,
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv)

    assert mock_process.call_count == 2
    assert mock_process.call_args_list[0][0][0] == SceneSplitMessage(
        job_id="1", storage_url="/fake/a.mp4", target_resolution="480p"
    )
    assert mock_process.call_args_list[1][0][0] == SceneSplitMessage(
        job_id="2", storage_url="/fake/b.mp4", target_resolution="480p"
    )


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
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job",
            new_callable=AsyncMock,
            return_value=chunk_messages,
        ),
        patch(
            "src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock
        ) as mock_publish,
    ):
        await raw_videos(mock_js, mock_kv)

    mock_publish.assert_called_once_with(mock_js, chunk_messages)


@pytest.mark.asyncio
async def test_acks_and_skips_when_job_already_processed() -> None:
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv(already_processed=True)

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ) as mock_process,
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    mock_process.assert_not_called()


@pytest.mark.asyncio
async def test_writes_to_kv_on_success() -> None:
    msg = make_mock_msg(
        {
            "job_id": "abc-123",
            "storage_url": "/fake/idk.mp4",
            "target_resolution": "480p",
        }
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ),
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv)

    mock_kv.put.assert_called_once_with("abc-123", b"done")


@pytest.mark.asyncio
async def test_does_not_write_to_kv_when_process_job_fails() -> None:
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job",
            new_callable=AsyncMock,
            side_effect=Exception("failed"),
        ),
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js, mock_kv)

    mock_kv.put.assert_not_called()


@pytest.mark.asyncio
async def test_does_not_write_to_kv_when_publish_fails() -> None:
    msg = make_mock_msg(
        {"job_id": "1", "storage_url": "/fake/idk.mp4", "target_resolution": "480p"}
    )
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub
    mock_kv = make_mock_kv()

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ),
        patch(
            "src.nats.subscriber.scene_video_chunks",
            new_callable=AsyncMock,
            side_effect=Exception("publish failed"),
        ),
    ):
        await raw_videos(mock_js, mock_kv)

    mock_kv.put.assert_not_called()
