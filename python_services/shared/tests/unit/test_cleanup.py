from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from shared_util import cleanup_temp


@pytest.mark.asyncio
async def test_cleanup_temp_succeeds_first_try(tmp_path: Path) -> None:
    logger = MagicMock()
    target = str(tmp_path / "job-1")

    with patch("shared_util.cleanup._remove") as mock_remove:
        await cleanup_temp(target, "job-1", logger)

    mock_remove.assert_called_once_with(target)
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_returns_silently_on_missing_path(tmp_path: Path) -> None:
    logger = MagicMock()
    target = str(tmp_path / "job-1")

    with patch(
        "shared_util.cleanup._remove", side_effect=FileNotFoundError()
    ) as mock_remove:
        await cleanup_temp(target, "job-1", logger)

    mock_remove.assert_called_once()
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_retries_then_succeeds(tmp_path: Path) -> None:
    logger = MagicMock()
    target = str(tmp_path / "job-1")

    with (
        patch(
            "shared_util.cleanup._remove",
            side_effect=[OSError("busy"), OSError("busy"), None],
        ) as mock_remove,
        patch("shared_util.cleanup.asyncio.sleep") as mock_sleep,
    ):
        await cleanup_temp(target, "job-1", logger, delay_seconds=0)

    assert mock_remove.call_count == 3
    assert mock_sleep.call_count == 2
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_logs_after_exhausting_retries(tmp_path: Path) -> None:
    logger = MagicMock()
    target = str(tmp_path / "job-1")

    with (
        patch(
            "shared_util.cleanup._remove", side_effect=OSError("busy")
        ) as mock_remove,
        patch("shared_util.cleanup.asyncio.sleep") as mock_sleep,
    ):
        await cleanup_temp(target, "job-1", logger, retries=3, delay_seconds=0)

    assert mock_remove.call_count == 3
    assert mock_sleep.call_count == 2
    logger.error.assert_called_once()
    assert logger.error.call_args.kwargs["temp_path"] == target
    assert logger.error.call_args.kwargs["job_id"] == "job-1"
    assert logger.error.call_args.kwargs["attempts"] == 3


@pytest.mark.asyncio
async def test_cleanup_temp_removes_directory_tree(tmp_path: Path) -> None:
    logger = MagicMock()
    job_dir = tmp_path / "job-1"
    (job_dir / "nested").mkdir(parents=True)
    (job_dir / "nested" / "chunk.mp4").write_bytes(b"x")

    await cleanup_temp(str(job_dir), "job-1", logger)

    assert not job_dir.exists()
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_removes_single_file(tmp_path: Path) -> None:
    logger = MagicMock()
    temp_file = tmp_path / "upscaled_noaudio-job-1.mp4"
    temp_file.write_bytes(b"x")

    await cleanup_temp(str(temp_file), "job-1", logger)

    assert not temp_file.exists()
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_removes_symlink_without_touching_target(
    tmp_path: Path,
) -> None:
    logger = MagicMock()
    real_dir = tmp_path / "real"
    real_dir.mkdir()
    (real_dir / "keep.txt").write_text("keep")
    link = tmp_path / "link"
    link.symlink_to(real_dir, target_is_directory=True)

    await cleanup_temp(str(link), "job-1", logger)

    assert not link.is_symlink()
    assert (real_dir / "keep.txt").exists()
    logger.error.assert_not_called()


@pytest.mark.asyncio
async def test_cleanup_temp_missing_real_path_is_silent(tmp_path: Path) -> None:
    logger = MagicMock()

    await cleanup_temp(str(tmp_path / "does-not-exist"), "job-1", logger)

    logger.error.assert_not_called()
