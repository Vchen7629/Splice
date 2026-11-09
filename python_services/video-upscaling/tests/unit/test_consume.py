import json
from pathlib import Path
from typing import Any, AsyncGenerator
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from nats.aio.msg import Msg

from core import JobCancelledError, VideoNotFoundErr, input_dir, output_dir
from msgbus import JobContext
from msgbus.consume import _cleanup_job, _handle_consumer_message, consumer
from tests.fixtures.nats import make_msg


async def async_iter(items: Any) -> AsyncGenerator[Any, None]:
    for item in items:
        yield item


def make_mock_sub(*msgs: AsyncMock) -> MagicMock:
    sub = MagicMock()
    sub.messages = async_iter(list(msgs))
    return sub


@pytest.mark.asyncio
async def test_consumer_calls_handle_consumer_message_once_per_message(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    mock_sub = make_mock_sub(make_msg(), make_msg())

    with patch(
        "msgbus.consume._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(job_ctx, mock_sub, processed_kv, mock_logger, AsyncMock())

    assert mock_handle.call_count == 2


@pytest.mark.asyncio
async def test_consumer_passes_correct_args_to_handle_consumer_message(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg()
    mock_orchestrator = AsyncMock()

    with patch(
        "msgbus.consume._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(
            job_ctx, make_mock_sub(msg), processed_kv, mock_logger, mock_orchestrator
        )

    mock_handle.assert_called_once_with(
        job_ctx, processed_kv, msg, mock_logger, mock_orchestrator
    )


@pytest.mark.asyncio
async def test_consumer_terminates_and_skips_cancelled_job(
    job_ctx: JobContext,
    processed_kv: AsyncMock,
    mock_logger: MagicMock,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    msg = make_msg(job_id="job-skip-1")
    monkeypatch.setattr("msgbus.consume.is_job_cancelled", AsyncMock(return_value=True))

    with patch(
        "msgbus.consume._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(
            job_ctx, make_mock_sub(msg), processed_kv, mock_logger, AsyncMock()
        )

    msg.term.assert_called_once()
    mock_handle.assert_not_called()


@pytest.mark.asyncio
async def test_consumer_job_id_empty_string_skips_is_job_cancelled(
    job_ctx: JobContext,
    processed_kv: AsyncMock,
    mock_logger: MagicMock,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # raw payload, an empty job_id isn't a valid ProcessJobMessage
    msg = AsyncMock(spec=Msg)
    msg.data = json.dumps({"job_id": ""}).encode()
    mock_is_job_cancelled = AsyncMock(return_value=False)
    monkeypatch.setattr("msgbus.consume.is_job_cancelled", mock_is_job_cancelled)

    with patch(
        "msgbus.consume._handle_consumer_message", new_callable=AsyncMock
    ) as mock_handle:
        await consumer(
            job_ctx, make_mock_sub(msg), processed_kv, mock_logger, AsyncMock()
        )

    msg.term.assert_not_called()
    mock_is_job_cancelled.assert_not_called()
    mock_handle.assert_called()


@pytest.mark.asyncio
async def test_acks_on_success(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg()

    await _handle_consumer_message(job_ctx, processed_kv, msg, mock_logger, AsyncMock())

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_acks_and_skips_when_job_already_processed(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg()
    processed_kv.get.side_effect = None
    processed_kv.get.return_value = MagicMock()
    mock_orchestrator = AsyncMock()

    await _handle_consumer_message(
        job_ctx, processed_kv, msg, mock_logger, mock_orchestrator
    )

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    mock_orchestrator.assert_not_called()


@pytest.mark.asyncio
async def test_writes_msg_processed_kv_on_success(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg(job_id="abc-123")

    await _handle_consumer_message(job_ctx, processed_kv, msg, mock_logger, AsyncMock())

    processed_kv.put.assert_called_once_with("abc-123", b"done")


@pytest.mark.asyncio
async def test_stage_updated_before_job_orchestrator_runs(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg(job_id="abc-123")
    call_order: list[str] = []

    async def fake_orchestrator(*_args: Any) -> None:
        call_order.append("job_orchestrator")

    async def fake_update(key: str, value: bytes, last: int) -> None:
        call_order.append("job_status_update")

    job_ctx.milestone_kv.update.side_effect = fake_update

    await _handle_consumer_message(
        job_ctx, processed_kv, msg, mock_logger, fake_orchestrator
    )

    assert call_order == ["job_status_update", "job_orchestrator"]


@pytest.mark.asyncio
async def test_updates_kv_and_acks_on_failure(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg(job_id="1")
    mock_orchestrator = AsyncMock(side_effect=Exception("process failed"))

    await _handle_consumer_message(
        job_ctx, processed_kv, msg, mock_logger, mock_orchestrator
    )

    job_id, payload = job_ctx.milestone_kv.update.call_args_list[-1][0]
    assert job_id == "1"
    assert json.loads(payload)["state"] == "FAILED"
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_does_not_write_msg_processed_kv_on_failure(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    mock_orchestrator = AsyncMock(side_effect=Exception("boom"))

    await _handle_consumer_message(
        job_ctx, processed_kv, make_msg(), mock_logger, mock_orchestrator
    )

    processed_kv.put.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_error_records_failure_and_acks(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    """When milestone_kv.update fails for the stage write but succeeds for the
    failed-write fallback, the failure is durably recorded and the message is acked."""
    msg = make_msg()
    job_ctx.milestone_kv.update.side_effect = [Exception("kv write failed"), None]

    await _handle_consumer_message(job_ctx, processed_kv, msg, mock_logger, AsyncMock())

    assert job_ctx.milestone_kv.update.call_count == 2
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_error_naks_when_failure_write_also_fails(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    """When milestone_kv.update fails for both the stage write and the failed-write
    fallback, nothing was durably recorded, so the message is nak'd for redelivery."""
    msg = make_msg()
    job_ctx.milestone_kv.update.side_effect = Exception("kv write failed")

    await _handle_consumer_message(job_ctx, processed_kv, msg, mock_logger, AsyncMock())

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_acks_and_does_not_record_failure_when_job_cancelled(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    """When job_orchestrator raises JobCancelledError, the msg should be acked and
    not recorded as failure in milestone_kv"""
    msg = make_msg()
    mock_orchestrator = AsyncMock(
        side_effect=JobCancelledError("cancelled during processing")
    )

    await _handle_consumer_message(
        job_ctx, processed_kv, msg, mock_logger, mock_orchestrator
    )

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    job_ctx.milestone_kv.update.assert_called_once()


@pytest.mark.asyncio
async def test_cleanup_job_called_on_success(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg(job_id="abc-123")

    with patch("msgbus.consume._cleanup_job", new_callable=AsyncMock) as mock_cleanup:
        await _handle_consumer_message(
            job_ctx, processed_kv, msg, mock_logger, AsyncMock()
        )

    mock_cleanup.assert_called_once_with("abc-123", mock_logger)


@pytest.mark.asyncio
async def test_cleanup_job_called_on_failure_before_nak(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    msg = make_msg(job_id="abc-123")
    job_ctx.milestone_kv.update.side_effect = Exception("kv write failed")
    call_order: list[str] = []

    async def fake_cleanup(*_args: Any) -> None:
        call_order.append("cleanup_job")

    async def fake_nak() -> None:
        call_order.append("nak")

    msg.nak.side_effect = fake_nak

    with patch("msgbus.consume._cleanup_job", side_effect=fake_cleanup):
        await _handle_consumer_message(
            job_ctx, processed_kv, msg, mock_logger, AsyncMock()
        )

    assert call_order == ["cleanup_job", "nak"]


@pytest.mark.asyncio
async def test_video_not_found_records_failed_milestone_and_acks(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    """A missing/expired artifact writes a FAILED milestone with a clear error
    and the message is acked (terminal, no redelivery) once the write succeeds"""
    msg = make_msg(job_id="1")
    mock_orchestrator = AsyncMock(side_effect=VideoNotFoundErr("gone"))

    await _handle_consumer_message(
        job_ctx, processed_kv, msg, mock_logger, mock_orchestrator
    )

    job_id, payload = job_ctx.milestone_kv.update.call_args_list[-1][0]
    assert job_id == "1"
    assert json.loads(payload)["state"] == "FAILED"
    assert "missing or expired" in payload.decode()
    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
    processed_kv.put.assert_not_called()


@pytest.mark.asyncio
async def test_video_not_found_naks_when_failed_milestone_write_fails(
    job_ctx: JobContext, processed_kv: AsyncMock, mock_logger: MagicMock
) -> None:
    """If the FAILED milestone can't be persisted, the message must stay
    retryable: nak'd and never acked"""
    msg = make_msg()
    # stage write succeeds, FAILED write fails
    job_ctx.milestone_kv.update.side_effect = [None, Exception("kv write failed")]
    mock_orchestrator = AsyncMock(side_effect=VideoNotFoundErr("gone"))

    await _handle_consumer_message(
        job_ctx, processed_kv, msg, mock_logger, mock_orchestrator
    )

    msg.nak.assert_called_once()
    msg.ack.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_job_removes_input_and_output_dirs(
    mock_logger: MagicMock,
) -> None:
    for d in (Path(input_dir("job-1")), Path(output_dir("job-1"))):
        (d / "nested").mkdir(parents=True)
        (d / "nested" / "video.mp4").write_bytes(b"x")

    await _cleanup_job("job-1", mock_logger)

    assert not Path(input_dir("job-1")).exists()
    assert not Path(output_dir("job-1")).exists()
    mock_logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_job_missing_dirs_is_silent(mock_logger: MagicMock) -> None:
    await _cleanup_job("does-not-exist", mock_logger)

    mock_logger.error.assert_not_called()
