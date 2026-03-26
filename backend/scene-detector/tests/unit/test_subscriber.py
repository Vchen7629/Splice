from typing import Any, AsyncGenerator
from unittest.mock import patch, MagicMock, AsyncMock
from nats.js.errors import APIError
from nats.js.client import JetStreamContext
from src.nats.subscriber import raw_videos
from src.nats.messages import SceneSplitMessage, VideoChunkMessage
import json
import pytest


def make_mock_msg(data: dict[str, Any]) -> AsyncMock:
    msg = AsyncMock()
    msg.data = json.dumps(data).encode()
    return msg


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


@pytest.mark.asyncio
async def test_acks_on_success() -> None:
    msg = make_mock_msg({"job_id": "1", "storage_path": "/fake/idk.mp4"})
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ),
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_naks_when_process_job_fails() -> None:
    msg = make_mock_msg({"job_id": "1", "storage_path": "/fake/idk.mp4"})
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub

    with (
        patch(
            "src.nats.subscriber.process_job",
            new_callable=AsyncMock,
            side_effect=Exception("failed"),
        ),
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_naks_when_publish_fails() -> None:
    msg = make_mock_msg({"job_id": "1", "storage_path": "/fake/idk.mp4"})
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub

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
        await raw_videos(mock_js)

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_raises_when_subscribe_fails() -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.side_effect = APIError()

    with pytest.raises(APIError):
        await raw_videos(mock_js)


@pytest.mark.asyncio
async def test_calls_process_job_per_message() -> None:
    msgs = [
        make_mock_msg({"job_id": "1", "storage_path": "/fake/a.mp4"}),
        make_mock_msg({"job_id": "2", "storage_path": "/fake/b.mp4"}),
    ]
    mock_sub = MagicMock()
    mock_sub.messages = async_iter(msgs)
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub

    with (
        patch(
            "src.nats.subscriber.process_job", new_callable=AsyncMock, return_value=[]
        ) as mock_process,
        patch("src.nats.subscriber.scene_video_chunks", new_callable=AsyncMock),
    ):
        await raw_videos(mock_js)

    assert mock_process.call_count == 2
    assert mock_process.call_args_list[0][0][0] == SceneSplitMessage(
        job_id="1", storage_path="/fake/a.mp4"
    )
    assert mock_process.call_args_list[1][0][0] == SceneSplitMessage(
        job_id="2", storage_path="/fake/b.mp4"
    )


@pytest.mark.asyncio
async def test_passes_chunk_messages_to_publisher() -> None:
    chunk_messages = [
        VideoChunkMessage(job_id="1", chunk_index=0, storage_path="/tmp/chunk-001.mp4")
    ]
    msg = make_mock_msg({"job_id": "1", "storage_path": "/fake/idk.mp4"})
    mock_sub = MagicMock()
    mock_sub.messages = async_iter([msg])
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.return_value = mock_sub

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
        await raw_videos(mock_js)

    mock_publish.assert_called_once_with(mock_js, chunk_messages)
