import asyncio
import json
from typing import Any, AsyncGenerator
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from nats.aio.client import Client as NATSClient
from nats.aio.msg import Msg
from nats.errors import AuthorizationError, NoServersError, TimeoutError
from nats.js.client import JetStreamContext
from nats.js.errors import KeyNotFoundError
from nats.js.kv import KeyValue
from structlog.stdlib import BoundLogger

from shared_handler import (
    JobCancelledError,
    JobMsgContext,
    check_cancel_event,
    consumer,
    nats_connect,
)
from shared_handler.nats import _handle_consumer_message
from test_helpers.nats import make_msg, milestone_entry

MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_KV = AsyncMock(spec=KeyValue)
MOCK_LOGGER = MagicMock(spec=BoundLogger)


def make_ctx(**overrides: Any) -> JobMsgContext:
    msg_processed_kv = AsyncMock(spec=KeyValue)
    msg_processed_kv.get.side_effect = KeyNotFoundError()
    job_milestone_kv = AsyncMock(spec=KeyValue)
    job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")

    defaults: dict[str, Any] = dict(
        nc=MOCK_NC,
        js=AsyncMock(spec=JetStreamContext),
        msg_processed_kv=msg_processed_kv,
        job_milestone_kv=job_milestone_kv,
        service_name="scene-detector",
        logger=MagicMock(spec=BoundLogger),
    )
    defaults.update(overrides)
    return JobMsgContext(**defaults)


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


def make_mock_sub(*msgs: AsyncMock) -> MagicMock:
    sub = MagicMock()
    sub.messages = async_iter(list(msgs))
    return sub


def make_mock_msg(job_id: str = "job-1") -> AsyncMock:
    msg = AsyncMock(spec=Msg)
    msg.data = json.dumps({"job_id": job_id}).encode()

    return msg


@pytest.mark.asyncio
@pytest.mark.parametrize(
    argnames="exc", argvalues=[NoServersError(), AuthorizationError(), TimeoutError()]
)
async def test_nats_connect_raises_on_nats_failure(exc: Any) -> None:
    """It should raise the error when caught"""
    with patch("shared_handler.nats.NATSClient") as mock_client_class:
        mock_instance = MagicMock(spec=NATSClient)
        mock_instance.connect = AsyncMock(side_effect=exc)
        mock_client_class.return_value = mock_instance
        with pytest.raises(type(exc)):
            await nats_connect(service_name="scene-detector")


@pytest.mark.asyncio
async def test_nats_connect_returns_nats_and_jetstream() -> None:
    mock_js = MagicMock(spec=JetStreamContext)
    mock_ns = MagicMock(spec=NATSClient)
    mock_ns.connect = AsyncMock()
    mock_ns.jetstream.return_value = mock_js

    with patch("shared_handler.nats.NATSClient", return_value=mock_ns):
        nc, js = await nats_connect(service_name="scene-detector")

    assert nc is mock_ns
    assert js is mock_js


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
async def test_consumer_calls_handle_consumer_message_once_per_message() -> None:
    msgs = [make_mock_msg(), make_mock_msg()]
    mock_sub = make_mock_sub(*msgs)
    ctx = make_ctx()

    with patch(
        "shared_handler.nats._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(ctx, mock_sub, AsyncMock())

    assert mock_handle.call_count == 2


@pytest.mark.asyncio
async def test_consumer_passes_correct_args_to_handle_consumer_message() -> None:
    msg = make_mock_msg()
    mock_sub = make_mock_sub(msg)
    ctx = make_ctx()
    mock_process_job_msg = AsyncMock()
    mock_cleanup_job = AsyncMock()

    with patch(
        "shared_handler.nats._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(ctx, mock_sub, mock_process_job_msg, mock_cleanup_job)

    mock_handle.assert_called_once_with(
        ctx, msg, mock_process_job_msg, mock_cleanup_job
    )


@pytest.mark.asyncio
async def test_consumer_terminates_and_skips_cancelled_job(monkeypatch) -> None:
    msg = make_mock_msg(job_id="job-skip-1")
    mock_sub = make_mock_sub(msg)
    ctx = make_ctx()

    monkeypatch.setattr(
        "shared_handler.nats.is_job_cancelled", AsyncMock(return_value=True)
    )

    with patch(
        "shared_handler.nats._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(ctx, mock_sub, AsyncMock())

    msg.term.assert_called_once()
    mock_handle.assert_not_called()


@pytest.mark.asyncio
async def test_consumer_job_id_empty_string_skips_is_job_cancelled(monkeypatch) -> None:
    msg = make_mock_msg(job_id="")
    mock_sub = make_mock_sub(msg)
    ctx = make_ctx()
    mock_is_job_cancelled = AsyncMock(return_value=False)

    monkeypatch.setattr("shared_handler.nats.is_job_cancelled", mock_is_job_cancelled)

    with patch(
        "shared_handler.nats._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(ctx, mock_sub, AsyncMock())

    msg.term.assert_not_called()
    mock_is_job_cancelled.assert_not_called()
    mock_handle.assert_called()


@pytest.mark.asyncio
async def test_acks_on_success() -> None:
    msg = make_msg()
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")

    await _handle_consumer_message(ctx, msg, AsyncMock())

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_acks_and_skips_when_job_already_processed() -> None:
    msg = make_msg()
    ctx = make_ctx()
    ctx.msg_processed_kv.get.side_effect = None
    ctx.msg_processed_kv.get.return_value = MagicMock()
    mock_process_job_msg = AsyncMock()

    await _handle_consumer_message(ctx, msg, mock_process_job_msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    mock_process_job_msg.assert_not_called()


@pytest.mark.asyncio
async def test_writes_msg_processed_kv_on_success() -> None:
    msg = make_msg(job_id="abc-123")
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")

    await _handle_consumer_message(ctx, msg, AsyncMock())

    ctx.msg_processed_kv.put.assert_called_once_with("abc-123", b"done")


@pytest.mark.asyncio
async def test_stage_updated_before_process_job_msg_runs() -> None:
    msg = make_msg(job_id="abc-123")
    ctx = make_ctx(service_name="scene-detector")
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    call_order: list[str] = []

    async def fake_process_job_msg(_ctx: Any, _cancel_event: Any) -> None:
        call_order.append("process_job_msg")

    async def fake_update(key: str, value: bytes, last: int) -> None:
        call_order.append("job_status_update")

    ctx.job_milestone_kv.update.side_effect = fake_update

    await _handle_consumer_message(ctx, msg, fake_process_job_msg)

    assert call_order == ["job_status_update", "process_job_msg"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "side_effect", [Exception("process failed")], ids=["process_job_msg_fails"]
)
async def test_updates_kv_and_acks_on_failure(side_effect: Exception) -> None:
    msg = make_msg(job_id="1")
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    mock_process_job_msg = AsyncMock(side_effect=side_effect)

    await _handle_consumer_message(ctx, msg, mock_process_job_msg)

    job_id, payload = ctx.job_milestone_kv.update.call_args_list[-1][0]
    assert job_id == "1"
    assert json.loads(payload)["state"] == "FAILED"
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_does_not_write_msg_processed_kv_on_failure() -> None:
    msg = make_msg()
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    mock_process_job_msg = AsyncMock(side_effect=Exception("boom"))

    await _handle_consumer_message(ctx, msg, mock_process_job_msg)

    ctx.msg_processed_kv.put.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_error_records_failure_and_acks() -> None:
    """When job_milestone_kv.update fails for the stage write but succeeds for the
    failed-write fallback, the failure is durably recorded and the message is acked."""
    msg = make_msg()
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    ctx.job_milestone_kv.update.side_effect = [Exception("kv write failed"), None]

    await _handle_consumer_message(ctx, msg, AsyncMock())

    assert ctx.job_milestone_kv.update.call_count == 2
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_error_naks_when_failure_write_also_fails() -> None:
    """When job_milestone_kv.update fails for both the stage write and the failed-write
    fallback, nothing was durably recorded, so the message is nak'd for redelivery."""
    msg = make_msg()
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    ctx.job_milestone_kv.update.side_effect = Exception("kv write failed")

    await _handle_consumer_message(ctx, msg, AsyncMock())

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_acks_and_does_not_record_failure_when_job_cancelled() -> None:
    """When process_job_msg raises JobCancelledError, the msg should be acked and
    not recorded as failure in job_milestone_kv"""
    msg = make_msg()
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    mock_process_job_msg = AsyncMock(
        side_effect=JobCancelledError("cancelled during processing")
    )

    await _handle_consumer_message(ctx, msg, mock_process_job_msg)

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    ctx.job_milestone_kv.update.assert_called_once()


@pytest.mark.asyncio
async def test_cleanup_job_called_on_success() -> None:
    msg = make_msg(job_id="abc-123")
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    mock_cleanup_job = AsyncMock()

    await _handle_consumer_message(ctx, msg, AsyncMock(), mock_cleanup_job)

    mock_cleanup_job.assert_called_once_with("abc-123")


@pytest.mark.asyncio
async def test_cleanup_job_called_on_failure_before_nak() -> None:
    msg = make_msg(job_id="abc-123")
    ctx = make_ctx()
    ctx.job_milestone_kv.get.return_value = milestone_entry("PROCESSING", "upload")
    ctx.job_milestone_kv.update.side_effect = Exception("kv write failed")
    call_order: list[str] = []

    async def fake_cleanup(_job_id: str) -> None:
        call_order.append("cleanup_job")

    async def fake_nak() -> None:
        call_order.append("nak")

    msg.nak.side_effect = fake_nak

    await _handle_consumer_message(ctx, msg, AsyncMock(), fake_cleanup)

    assert call_order == ["cleanup_job", "nak"]
