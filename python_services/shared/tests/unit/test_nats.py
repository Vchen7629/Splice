from typing import Any, AsyncGenerator
from unittest.mock import AsyncMock, MagicMock
from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.errors import APIError, KeyNotFoundError
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from shared_core import get_logger
from shared_handler import consumer, check_cancel_event
import pytest

MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_KV = AsyncMock(spec=KeyValue)


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


def make_mock_js(*msgs: AsyncMock) -> AsyncMock:
    js = AsyncMock(spec=JetStreamContext)
    sub = MagicMock()
    sub.messages = async_iter(list(msgs))
    js.subscribe.return_value = sub
    return js


def make_mock_msg(job_id: str = "job-1") -> AsyncMock:
    import json

    msg = AsyncMock(spec=Msg)
    msg.data = json.dumps({"job_id": job_id}).encode()

    return msg


@pytest.mark.asyncio
async def test_check_cancel_event_unsubscribes_on_exit() -> None:
    nc = AsyncMock(spec=NATSClient)
    sub = AsyncMock()
    nc.subscribe.return_value = sub
    logger = get_logger("test")

    async with check_cancel_event(nc, "job-2", logger):
        pass

    sub.unsubscribe.assert_awaited_once()


@pytest.mark.asyncio
async def test_consumer_calls_process_msg_once_per_message() -> None:
    msgs = [make_mock_msg(), make_mock_msg()]
    mock_js = make_mock_js(*msgs)
    mock_process_msg = AsyncMock()
    mock_job_milestone_kv = AsyncMock(spec=KeyValue)
    mock_job_milestone_kv.get.side_effect = KeyNotFoundError()

    await consumer(
        MOCK_NC,
        mock_js,
        MOCK_KV,
        mock_job_milestone_kv,
        "idk",
        "idk2",
        "idk2",
        mock_process_msg,
    )

    assert mock_process_msg.call_count == 2


@pytest.mark.asyncio
async def test_consumer_passes_correct_args_to_process_msg() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_job_status_kv = AsyncMock(spec=KeyValue)
    mock_job_status_kv.get.side_effect = KeyNotFoundError()
    msg = make_mock_msg()
    mock_js = make_mock_js(msg)
    mock_process_msg = make_mock_msg()

    await consumer(
        MOCK_NC,
        mock_js,
        mock_kv,
        mock_job_status_kv,
        "subject",
        "durable",
        "queue",
        mock_process_msg,
    )

    mock_process_msg.assert_called_once_with(
        MOCK_NC, mock_js, mock_kv, mock_job_status_kv, msg
    )


@pytest.mark.asyncio
async def test_consumer_raises_when_subscribe_fails() -> None:
    mock_js = AsyncMock(spec=JetStreamContext)
    mock_js.subscribe.side_effect = APIError()

    with pytest.raises(APIError):
        await consumer(
            MOCK_NC,
            mock_js,
            MOCK_KV,
            MOCK_KV,
            "idk1",
            "idk2",
            "idk2",
            AsyncMock(),
        )


@pytest.mark.asyncio
async def test_consumer_terminates_and_skips_cancelled_job(monkeypatch) -> None:
    msg = make_mock_msg(job_id="job-skip-1")
    mock_js = make_mock_js(msg)
    mock_process_msg = AsyncMock(spec=Msg)
    mock_job_milestone_kv = AsyncMock(spec=KeyValue)

    monkeypatch.setattr(
        "shared_handler.nats.is_job_cancelled", AsyncMock(return_value=True)
    )

    await consumer(
        MOCK_NC,
        mock_js,
        MOCK_KV,
        mock_job_milestone_kv,
        "subject",
        "durable",
        "queue",
        mock_process_msg,
    )

    msg.term.assert_called_once()
    mock_process_msg.assert_not_called()


@pytest.mark.asyncio
async def test_consumer_job_id_empty_string_skips_is_job_cancelled(monkeypatch) -> None:
    msg = make_mock_msg(job_id="")
    mock_js = make_mock_js(msg)
    mock_process_msg = AsyncMock(spec=Msg)
    mock_job_milestone_kv = AsyncMock(spec=KeyValue)
    mock_is_job_cancelled = AsyncMock(return_value=False)

    monkeypatch.setattr("shared_handler.nats.is_job_cancelled", mock_is_job_cancelled)

    await consumer(
        MOCK_NC,
        mock_js,
        MOCK_KV,
        mock_job_milestone_kv,
        "subject",
        "durable",
        "queue",
        mock_process_msg,
    )

    msg.term.assert_not_called()
    mock_is_job_cancelled.assert_not_called()
    mock_process_msg.assert_called()
