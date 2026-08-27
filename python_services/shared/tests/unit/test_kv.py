from unittest.mock import AsyncMock, MagicMock
from nats.js.errors import KeyNotFoundError, KeyWrongLastSequenceError
from nats.js.kv import KeyValue
from shared_handler.kv import advance_milestone
from test_helpers.nats import milestone_entry
import pytest


mock_kv = AsyncMock(spec=KeyValue)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "current_entry,new_stage,should_write",
    [
        (milestone_entry("PROCESSING", "video-upscaling"), "video-recombiner", True),
        (milestone_entry("PROCESSING", "video-recombiner"), "video-upscaling", False),
        (milestone_entry("COMPLETE"), "video-upscaling", False),
        (milestone_entry("FAILED"), "video-upscaling", False),
    ],
    ids=[
        "advances-forward-stage",
        "no-op-behind-stage",
        "no-op-terminal-complete",
        "no-op-terminal-failed",
    ],
)
async def test_advance_milestone_write_policy(
    current_entry: MagicMock, new_stage: str, should_write: bool
) -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.return_value = current_entry

    await advance_milestone(
        mock_kv, "job-1", {"state": "PROCESSING", "stage": new_stage}
    )

    if should_write:
        mock_kv.update.assert_called_once()
    else:
        mock_kv.update.assert_not_called()


@pytest.mark.asyncio
async def test_advance_milestone_retries_on_revision_conflict() -> None:
    mock_kv.get.side_effect = [
        milestone_entry("PROCESSING", "video-upscaling", revision=1),
        milestone_entry("PROCESSING", "video-upscaling", revision=2),
    ]
    mock_kv.update.side_effect = [
        KeyWrongLastSequenceError(description="wrong last sequence"),
        3,
    ]

    await advance_milestone(
        mock_kv, "job-1", {"state": "PROCESSING", "stage": "video-recombiner"}
    )

    assert mock_kv.get.call_count == 2
    assert mock_kv.update.call_count == 2


@pytest.mark.asyncio
async def test_advance_milestone_propagates_get_failure() -> None:
    mock_kv = AsyncMock(spec=KeyValue)
    mock_kv.get.side_effect = KeyNotFoundError()

    with pytest.raises(KeyNotFoundError):
        await advance_milestone(
            mock_kv, "job-1", {"state": "PROCESSING", "stage": "video-upscaling"}
        )
