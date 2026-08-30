from unittest.mock import MagicMock, patch
from shared_util import cleanup_temp_dir, cleanup_temp_file
import pytest


@pytest.mark.asyncio
async def test_cleanup_temp_dir_succeeds_first_try() -> None:
    logger = MagicMock()

    with patch("shared_util.cleanup.rmtree") as mock_rmtree:
        await cleanup_temp_dir("../temp/job-1", "job-1", logger)

    mock_rmtree.assert_called_once_with("../temp/job-1")
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_dir_returns_silently_on_missing_dir() -> None:
    logger = MagicMock()

    with patch(
        "shared_util.cleanup.rmtree", side_effect=FileNotFoundError()
    ) as mock_rmtree:
        await cleanup_temp_dir("../temp/job-1", "job-1", logger)

    mock_rmtree.assert_called_once()
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_dir_retries_then_succeeds() -> None:
    logger = MagicMock()

    with (
        patch(
            "shared_util.cleanup.rmtree",
            side_effect=[OSError("busy"), OSError("busy"), None],
        ) as mock_rmtree,
        patch("shared_util.cleanup.asyncio.sleep") as mock_sleep,
    ):
        await cleanup_temp_dir("../temp/job-1", "job-1", logger, delay_seconds=0)

    assert mock_rmtree.call_count == 3
    assert mock_sleep.call_count == 2
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_dir_logs_after_exhausting_retries() -> None:
    logger = MagicMock()

    with (
        patch("shared_util.cleanup.rmtree", side_effect=OSError("busy")) as mock_rmtree,
        patch("shared_util.cleanup.asyncio.sleep") as mock_sleep,
    ):
        await cleanup_temp_dir(
            "../temp/job-1", "job-1", logger, retries=3, delay_seconds=0
        )

    assert mock_rmtree.call_count == 3
    assert mock_sleep.call_count == 2
    logger.error.assert_called_once()
    assert logger.error.call_args.kwargs["temp_dir"] == "../temp/job-1"
    assert logger.error.call_args.kwargs["job_id"] == "job-1"
    assert logger.error.call_args.kwargs["attempts"] == 3


@pytest.mark.asyncio
async def test_cleanup_temp_file_succeeds_first_try() -> None:
    logger = MagicMock()

    with patch("shared_util.cleanup.remove") as mock_remove:
        await cleanup_temp_file("/tmp/upscaled_noaudio-job-1.mp4", "job-1", logger)

    mock_remove.assert_called_once_with("/tmp/upscaled_noaudio-job-1.mp4")
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_file_returns_silently_on_missing_file() -> None:
    logger = MagicMock()

    with patch(
        "shared_util.cleanup.remove", side_effect=FileNotFoundError()
    ) as mock_remove:
        await cleanup_temp_file("/tmp/upscaled_noaudio-job-1.mp4", "job-1", logger)

    mock_remove.assert_called_once()
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_file_retries_then_succeeds() -> None:
    logger = MagicMock()

    with (
        patch(
            "shared_util.cleanup.remove",
            side_effect=[OSError("busy"), None],
        ) as mock_remove,
        patch("shared_util.cleanup.asyncio.sleep") as mock_sleep,
    ):
        await cleanup_temp_file(
            "/tmp/upscaled_noaudio-job-1.mp4", "job-1", logger, delay_seconds=0
        )

    assert mock_remove.call_count == 2
    assert mock_sleep.call_count == 1
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_file_logs_after_exhausting_retries() -> None:
    logger = MagicMock()

    with (
        patch("shared_util.cleanup.remove", side_effect=OSError("busy")) as mock_remove,
        patch("shared_util.cleanup.asyncio.sleep") as mock_sleep,
    ):
        await cleanup_temp_file(
            "/tmp/upscaled_noaudio-job-1.mp4",
            "job-1",
            logger,
            retries=3,
            delay_seconds=0,
        )

    assert mock_remove.call_count == 3
    assert mock_sleep.call_count == 2
    logger.error.assert_called_once()
    assert (
        logger.error.call_args.kwargs["temp_file"] == "/tmp/upscaled_noaudio-job-1.mp4"
    )
    assert logger.error.call_args.kwargs["job_id"] == "job-1"
    assert logger.error.call_args.kwargs["attempts"] == 3
