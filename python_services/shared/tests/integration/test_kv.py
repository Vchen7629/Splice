from typing import Any
from nats.js.client import JetStreamContext
from shared_handler import advance_milestone
import json
import pytest


@pytest.mark.asyncio
async def test_advance_milestone_does_not_regress_terminal_state(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """A late/retried write must not overwrite a job's terminal milestone, and must
    make zero writes when it doesn't"""
    _, js = js_context
    kv = await js.key_value("job-milestones")

    job_id = "job-retry-after-complete"
    complete_status = json.dumps({"state": "COMPLETE"}).encode()
    seed_revision = await kv.put(job_id, complete_status)

    await advance_milestone(
        kv, job_id, {"state": "PROCESSING", "stage": "video-upscaling"}
    )

    entry = await kv.get(job_id)
    assert entry.value is not None
    assert json.loads(entry.value)["state"] == "COMPLETE"
    assert entry.revision == seed_revision


@pytest.mark.asyncio
async def test_advance_milestone_advances_forward_stage(
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

    await advance_milestone(
        kv, job_id, {"state": "PROCESSING", "stage": "video-recombiner"}
    )

    entry = await kv.get(job_id)
    assert entry.value is not None
    payload = json.loads(entry.value)
    assert payload == {"state": "PROCESSING", "stage": "video-recombiner"}
    assert entry.revision != seed_revision
