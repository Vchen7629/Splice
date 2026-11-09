import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest
from nats.js.errors import KeyNotFoundError, KeyWrongLastSequenceError
from nats.js.kv import KeyValue

from msgbus.milestone import is_job_cancelled, update_job_stage, watch_for_cancel
from tests.fixtures.nats import milestone_entry


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "current_entry,new_stage,should_write",
    [
        (milestone_entry("PROCESSING", "video-upscaling"), "video-recombiner", True),
        (milestone_entry("PROCESSING", "video-recombiner"), "video-upscaling", False),
        (milestone_entry("COMPLETE"), "video-upscaling", False),
        (milestone_entry("FAILED"), "video-upscaling", False),
        (milestone_entry("CANCELLED"), "video-upscaling", False),
    ],
    ids=[
        "advances-forward-stage",
        "no-op-behind-stage",
        "no-op-terminal-complete",
        "no-op-terminal-failed",
        "no-op-terminal-cancelled",
    ],
)
async def test_update_job_stage_write_policy(
    current_entry: MagicMock, new_stage: str, should_write: bool
) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.return_value = current_entry

    await update_job_stage(mock_kv, "job-1", new_stage)

    if should_write:
        mock_kv.update.assert_called_once()
    else:
        mock_kv.update.assert_not_called()


@pytest.mark.asyncio
async def test_update_job_stage_retries_on_revision_conflict() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.side_effect = [
        milestone_entry("PROCESSING", "video-upscaling", revision=1),
        milestone_entry("PROCESSING", "video-upscaling", revision=2),
    ]
    mock_kv.update.side_effect = [
        KeyWrongLastSequenceError(description="wrong last sequence"),
        3,
    ]

    await update_job_stage(mock_kv, "job-1", "video-recombiner")

    assert mock_kv.get.call_count == 2
    assert mock_kv.update.call_count == 2


@pytest.mark.asyncio
async def test_update_job_stage_propagates_get_failure() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.side_effect = KeyNotFoundError()

    with pytest.raises(KeyNotFoundError):
        await update_job_stage(mock_kv, "job-1", "video-upscaling")


@pytest.mark.asyncio
async def test_is_job_cancelled_returns_true_on_cancelled() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.return_value = milestone_entry("CANCELLED")

    assert await is_job_cancelled(mock_kv, "job-1") is True


@pytest.mark.asyncio
async def test_is_job_cancelled_returns_false_when_key_not_found() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.side_effect = KeyNotFoundError()

    assert await is_job_cancelled(mock_kv, "job-1") is False


@pytest.mark.asyncio
async def test_is_job_cancelled_returns_false_when_entry_value_is_none() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.return_value = MagicMock(value=None)

    assert await is_job_cancelled(mock_kv, "job-1") is False


@pytest.mark.asyncio
async def test_is_job_cancelled_returns_false_on_processing() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.return_value = milestone_entry("PROCESSING")

    assert await is_job_cancelled(mock_kv, "job-1") is False


@pytest.mark.asyncio
async def test_watch_for_cancel_sets_event_when_job_cancelled(
    monkeypatch: pytest.MonkeyPatch, mock_logger: MagicMock
) -> None:
    monkeypatch.setattr(
        "msgbus.milestone.is_job_cancelled", AsyncMock(return_value=True)
    )

    async with watch_for_cancel(
        AsyncMock(spec=KeyValue), "job-1", mock_logger, interval_s=0.01
    ) as cancel_event:
        await asyncio.sleep(0.05)
        assert cancel_event.is_set()


@pytest.mark.asyncio
async def test_watch_for_cancel_stays_unset_when_job_not_cancelled(
    monkeypatch: pytest.MonkeyPatch, mock_logger: MagicMock
) -> None:
    monkeypatch.setattr(
        "msgbus.milestone.is_job_cancelled", AsyncMock(return_value=False)
    )

    async with watch_for_cancel(
        AsyncMock(spec=KeyValue), "job-2", mock_logger, interval_s=0.01
    ) as cancel_event:
        await asyncio.sleep(0.05)
        assert not cancel_event.is_set()


@pytest.mark.asyncio
async def test_watch_for_cancel_cancels_poll_task_on_exit(
    monkeypatch: pytest.MonkeyPatch, mock_logger: MagicMock
) -> None:
    monkeypatch.setattr(
        "msgbus.milestone.is_job_cancelled", AsyncMock(return_value=False)
    )

    async with watch_for_cancel(
        AsyncMock(spec=KeyValue), "job-3", mock_logger, interval_s=0.01
    ) as cancel_event:
        pass

    assert not cancel_event.is_set()
    tasks = [t for t in asyncio.all_tasks() if t is not asyncio.current_task()]
    assert not any("_poll" in (t.get_coro().__qualname__ or "") for t in tasks)


@pytest.mark.asyncio
async def test_watch_for_cancel_retries_after_transient_kv_error(
    monkeypatch: pytest.MonkeyPatch, mock_logger: MagicMock
) -> None:
    mock_is_job_cancelled = AsyncMock(
        side_effect=[Exception("transient kv error"), True]
    )
    monkeypatch.setattr("msgbus.milestone.is_job_cancelled", mock_is_job_cancelled)

    async with watch_for_cancel(
        AsyncMock(spec=KeyValue), "job-5", mock_logger, interval_s=0.01
    ) as cancel_event:
        await asyncio.sleep(0.05)
        assert cancel_event.is_set()

    assert mock_is_job_cancelled.call_count >= 2
    mock_logger.warning.assert_called_once()


@pytest.mark.asyncio
async def test_watch_for_cancel_polls_at_given_interval(
    monkeypatch: pytest.MonkeyPatch, mock_logger: MagicMock
) -> None:
    mock_is_job_cancelled = AsyncMock(return_value=False)
    monkeypatch.setattr("msgbus.milestone.is_job_cancelled", mock_is_job_cancelled)

    async with watch_for_cancel(
        AsyncMock(spec=KeyValue), "job-4", mock_logger, interval_s=0.01
    ):
        await asyncio.sleep(0.055)

    assert mock_is_job_cancelled.call_count >= 4
