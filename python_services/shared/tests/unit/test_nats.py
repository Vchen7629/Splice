import asyncio
from typing import Any, AsyncGenerator
from unittest.mock import AsyncMock, MagicMock

import pytest
from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.js.client import JetStreamContext
from nats.js.errors import APIError, KeyNotFoundError
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from shared_handler import check_cancel_event, consumer

MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_KV = AsyncMock(spec=KeyValue)
MOCK_LOGGER = MagicMock(spec=BoundLogger)


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
async def test_check_cancel_event_sets_event_when_job_cancelled(monkeypatch) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    monkeypatch.setattr(
        "shared_handler.nats.is_job_cancelled", AsyncMock(return_value=True)
    )

    async with check_cancel_event(
        mock_kv, "job-1", MOCK_LOGGER, interval_s=0.01
    ) as cancel_event:
        await asyncio.sleep(0.05)
        assert cancel_event.is_set()


@pytest.mark.asyncio
async def test_check_cancel_event_stays_unset_when_job_not_cancelled(
    monkeypatch,
) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    monkeypatch.setattr(
        "shared_handler.nats.is_job_cancelled", AsyncMock(return_value=False)
    )

    async with check_cancel_event(
        mock_kv, "job-2", MOCK_LOGGER, interval_s=0.01
    ) as cancel_event:
        await asyncio.sleep(0.05)
        assert not cancel_event.is_set()


@pytest.mark.asyncio
async def test_check_cancel_event_cancels_poll_task_on_exit(monkeypatch) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_is_job_cancelled = AsyncMock(return_value=False)
    monkeypatch.setattr("shared_handler.nats.is_job_cancelled", mock_is_job_cancelled)

    async with check_cancel_event(
        mock_kv, "job-3", MOCK_LOGGER, interval_s=0.01
    ) as cancel_event:
        pass

    assert not cancel_event.is_set()
    tasks = [t for t in asyncio.all_tasks() if t is not asyncio.current_task()]
    assert not any("_poll" in (t.get_coro().__qualname__ or "") for t in tasks)


@pytest.mark.asyncio
async def test_check_cancel_event_retries_after_transient_kv_error(monkeypatch) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_logger = MagicMock(spec=BoundLogger)
    mock_is_job_cancelled = AsyncMock(
        side_effect=[Exception("transient kv error"), True]
    )
    monkeypatch.setattr("shared_handler.nats.is_job_cancelled", mock_is_job_cancelled)

    async with check_cancel_event(
        mock_kv, "job-5", mock_logger, interval_s=0.01
    ) as cancel_event:
        await asyncio.sleep(0.05)
        assert cancel_event.is_set()

    assert mock_is_job_cancelled.call_count >= 2
    mock_logger.warning.assert_called_once()


@pytest.mark.asyncio
async def test_check_cancel_event_polls_at_given_interval(monkeypatch) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_is_job_cancelled = AsyncMock(return_value=False)
    monkeypatch.setattr("shared_handler.nats.is_job_cancelled", mock_is_job_cancelled)

    async with check_cancel_event(mock_kv, "job-4", MOCK_LOGGER, interval_s=0.01):
        await asyncio.sleep(0.055)

    assert mock_is_job_cancelled.call_count >= 4


@pytest.mark.asyncio
async def test_consumer_calls_process_msg_once_per_message() -> None:
    msgs = [make_mock_msg(), make_mock_msg()]
    mock_js = make_mock_js(*msgs)
    mock_process_msg = AsyncMock()
    mock_job_milestone_kv = AsyncMock(spec=KeyValue)
    mock_job_milestone_kv.get.side_effect = KeyNotFoundError()

    await consumer(
        MOCK_LOGGER,
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
        MOCK_LOGGER,
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
            MOCK_LOGGER,
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
        MOCK_LOGGER,
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
        MOCK_LOGGER,
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
