from nats.aio.client import Client as NATSClient
from nats.js.client import JetStreamContext
from nats.js.kv import KeyValue
from pathlib import Path
from src.core.settings import settings
from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch
from tests.unit.test_nats_msg import make_msg
from src.utils import ProgressReporter
from src.processing.nats_msg import process_msg
import asyncio
import concurrent.futures
import pytest


MOCK_NC = AsyncMock(spec=NATSClient)
MOCK_JS = AsyncMock(spec=JetStreamContext)
MOCK_KV = AsyncMock(spec=KeyValue)


def test_progress_reporter_publishes_first_reading() -> None:
    import json

    mock_nc = MagicMock(spec=NATSClient)

    with patch(
        "src.utils.progress_reporter.asyncio.run_coroutine_threadsafe"
    ) as mock_schedule:
        reporter = ProgressReporter(mock_nc, "job-1", MagicMock())
        reporter(10)

    mock_nc.publish.assert_called_once()
    subject, payload = mock_nc.publish.call_args.args
    assert subject == "progress.job-1"
    assert json.loads(payload) == {
        "job_id": "job-1",
        "stage": settings.SERVICE_NAME,
        "progress": 10,
    }
    mock_schedule.assert_called_once()


def test_progress_reporter_throttles_unchanged_percent() -> None:
    """progress reporter shouldnt change if percent doesnt update"""
    mock_nc = MagicMock(spec=NATSClient)

    with patch("src.utils.progress_reporter.asyncio.run_coroutine_threadsafe"):
        reporter = ProgressReporter(mock_nc, "job-1", AsyncMock())
        reporter(10)
        reporter(10)  # 2nd unchanged call

    assert mock_nc.publish.call_count == 1


def test_progress_reporter_publishes_on_percent_change() -> None:
    """progress reporter should publish if percent changes"""
    mock_nc = MagicMock(spec=NATSClient)

    with patch("src.utils.progress_reporter.asyncio.run_coroutine_threadsafe"):
        reporter = ProgressReporter(mock_nc, "job-1", AsyncMock())
        reporter(10)
        reporter(11)

    assert mock_nc.publish.call_count == 2


@pytest.mark.asyncio
async def test_progress_reporter_flush_awaits_all_pending_futures() -> None:
    """flush should await every future scheduled since the last flush"""
    mock_nc = MagicMock(spec=NATSClient)
    loop = asyncio.get_running_loop()
    futures = [concurrent.futures.Future(), concurrent.futures.Future()]
    scheduled = iter(futures)

    with patch(
        "src.utils.progress_reporter.asyncio.run_coroutine_threadsafe",
        side_effect=lambda coro, loop: next(scheduled),
    ):
        reporter = ProgressReporter(mock_nc, "job-1", loop)
        reporter(10)
        reporter(20)

    for fut in futures:
        fut.set_result(None)

    await reporter.flush()  # should return promptly, not hang


@pytest.mark.asyncio
async def test_progress_reporter_flush_propagates_publish_failure() -> None:
    """a failed progress publish must surface to the caller of flush"""
    mock_nc = MagicMock(spec=NATSClient)
    loop = asyncio.get_running_loop()
    fut: concurrent.futures.Future = concurrent.futures.Future()
    fut.set_exception(RuntimeError("nats down"))

    with patch(
        "src.utils.progress_reporter.asyncio.run_coroutine_threadsafe", return_value=fut
    ):
        reporter = ProgressReporter(mock_nc, "job-1", loop)
        reporter(10)

    with pytest.raises(RuntimeError, match="nats down"):
        await reporter.flush()


@pytest.mark.asyncio
async def test_progress_reporter_flush_does_not_reawait_already_flushed_futures() -> (
    None
):
    """a second flush() with nothing new pending should be a no-op"""
    mock_nc = MagicMock(spec=NATSClient)
    loop = asyncio.get_running_loop()
    fut: concurrent.futures.Future = concurrent.futures.Future()
    fut.set_result(None)

    with patch(
        "src.utils.progress_reporter.asyncio.run_coroutine_threadsafe", return_value=fut
    ):
        reporter = ProgressReporter(mock_nc, "job-1", loop)
        reporter(10)

    await reporter.flush()
    await reporter.flush()  # would hang/error if it tried to reawait a consumed future


@pytest.mark.asyncio
async def test_recombiner_stage_transition_waits_for_progress_flush(
    nats_msg_patches: dict[str, Any],
) -> None:
    """process_msg must not advance to the video-recombiner stage until all
    queued progress updates from video_upscale have been flushed"""
    nats_msg_patches["select"].return_value = (Path("/weights/model.pth"), 2)
    call_order: list[str] = []

    async def fake_flush(self: ProgressReporter) -> None:
        call_order.append("flush")

    async def fake_update_stage(kv: Any, job_id: str, stage: str, service: str) -> None:
        if stage == "video-recombiner":
            call_order.append("update_stage:video-recombiner")

    nats_msg_patches["update_stage"].side_effect = fake_update_stage
    msg = make_msg()

    with patch("src.processing.nats_msg.ProgressReporter.flush", new=fake_flush):
        await process_msg(MOCK_NC, MOCK_JS, MOCK_KV, MOCK_KV, msg)

    assert call_order == ["flush", "update_stage:video-recombiner"]
