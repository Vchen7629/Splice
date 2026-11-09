import asyncio
import json
from typing import Any
from unittest.mock import MagicMock

import pytest
from nats.js.api import KeyValueConfig
from nats.js.client import JetStreamContext

from msgbus.milestone import update_job_stage, watch_for_cancel


@pytest.mark.asyncio
async def test_update_job_stage_does_not_regress_terminal_state(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """A late/retried write must not overwrite a job's terminal milestone, and must
    make zero writes when it doesn't"""
    _, js = js_context
    kv = await js.key_value("job-milestones")

    job_id = "job-retry-after-complete"
    complete_status = json.dumps({"state": "COMPLETE"}).encode()
    seed_revision = await kv.put(job_id, complete_status)

    await update_job_stage(kv, job_id, "video-upscaling")

    entry = await kv.get(job_id)
    assert entry.value is not None
    assert json.loads(entry.value)["state"] == "COMPLETE"
    assert entry.revision == seed_revision


@pytest.mark.asyncio
async def test_update_job_stage_advances_forward_stage(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """Sanity check that a genuine forward-progressing write still lands."""
    _, js = js_context
    kv = await js.key_value("job-milestones")

    job_id = "job-forward-progress"
    upscaling_status = json.dumps(
        {"state": "PROCESSING", "stage": "video-upscaling"}
    ).encode()
    seed_revision = await kv.put(job_id, upscaling_status)

    await update_job_stage(kv, job_id, "video-recombiner")

    entry = await kv.get(job_id)
    assert entry.value is not None
    payload = json.loads(entry.value)
    assert payload == {"state": "PROCESSING", "stage": "video-recombiner"}
    assert entry.revision != seed_revision


@pytest.mark.asyncio
async def test_watch_for_cancel_sets_cancel_event_when_kv_marks_cancelled(
    js_context: tuple[Any, JetStreamContext], mock_logger: MagicMock
) -> None:
    _, js = js_context
    milestone_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-check-cancel-event-1")
    )
    await milestone_kv.put("job-1", json.dumps({"state": "PROCESSING"}).encode())

    async with watch_for_cancel(
        milestone_kv, "job-1", mock_logger, interval_s=0.05
    ) as cancel_event:
        await milestone_kv.put("job-1", json.dumps({"state": "CANCELLED"}).encode())
        await asyncio.wait_for(asyncio.to_thread(cancel_event.wait), timeout=5)

    assert cancel_event.is_set()


@pytest.mark.asyncio
async def test_watch_for_cancel_stays_unset_when_kv_never_cancelled(
    js_context: tuple[Any, JetStreamContext], mock_logger: MagicMock
) -> None:
    _, js = js_context
    milestone_kv = await js.create_key_value(
        config=KeyValueConfig(bucket="test-check-cancel-event-2")
    )
    await milestone_kv.put("job-2", json.dumps({"state": "PROCESSING"}).encode())

    async with watch_for_cancel(
        milestone_kv, "job-2", mock_logger, interval_s=0.05
    ) as cancel_event:
        await asyncio.sleep(0.2)

    assert not cancel_event.is_set()
