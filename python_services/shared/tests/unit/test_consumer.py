from typing import Any, AsyncGenerator
from unittest.mock import AsyncMock, MagicMock
from nats.js.errors import APIError
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from shared_handler.nats import consumer
import pytest


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


def make_mock_js(*msgs: AsyncMock) -> AsyncMock:
    js = AsyncMock(spec=JetStreamContext)
    sub = MagicMock()
    sub.messages = async_iter(list(msgs))
    js.subscribe.return_value = sub
    return js


@pytest.mark.asyncio
async def test_calls_process_msg_once_per_message() -> None:
    msgs = [AsyncMock(), AsyncMock()]
    mock_js = make_mock_js(*msgs)
    mock_process_msg = AsyncMock()

    await consumer(
        mock_js,
        AsyncMock(spec=KeyValue),
        AsyncMock(spec=KeyValue),
        "idk",
        "idk2",
        "idk2",
        mock_process_msg,
    )

    assert mock_process_msg.call_count == 2


@pytest.mark.asyncio
async def test_passes_correct_args_to_process_msg() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_job_status_kv = AsyncMock(spec=KeyValue)
    msg = AsyncMock()
    mock_js = make_mock_js(msg)
    mock_process_msg = AsyncMock()

    await consumer(
        mock_js,
        mock_kv,
        mock_job_status_kv,
        "subject",
        "durable",
        "queue",
        mock_process_msg,
    )

    mock_process_msg.assert_called_once_with(mock_js, mock_kv, mock_job_status_kv, msg)


@pytest.mark.asyncio
async def test_raises_when_subscribe_fails() -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.side_effect = APIError()

    with pytest.raises(APIError):
        await consumer(
            mock_js,
            AsyncMock(spec=KeyValue),
            AsyncMock(spec=KeyValue),
            "idk1",
            "idk2",
            "idk2",
            AsyncMock(),
        )
