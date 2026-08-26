from typing import Any, Generator
from unittest.mock import patch, AsyncMock
import pytest


@pytest.fixture
def nats_msg_patches() -> Generator[dict[str, Any], Any, None]:
    """Patches all external dependencies used by process_msg / _finalize_job."""
    with (
        patch(
            "src.processing.nats_msg.check_already_processed", new_callable=AsyncMock
        ) as mock_check,
        patch(
            "src.processing.nats_msg.update_job_stage", new_callable=AsyncMock
        ) as mock_update_stage,
        patch(
            "src.processing.nats_msg.update_job_failed", new_callable=AsyncMock
        ) as mock_update_failed,
        patch(
            "src.processing.nats_msg.fetch_video", return_value="/tmp/job-123/video.mp4"
        ) as mock_fetch,
        patch("src.processing.nats_msg.select_model") as mock_select,
        patch("src.processing.nats_msg.video_upscale") as mock_upscale,
        patch("src.processing.nats_msg.video_downscale") as mock_downscale,
        patch("src.processing.nats_msg.upload_video") as mock_upload,
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock) as mock_pub,
        patch("src.processing.nats_msg.shutil.rmtree") as mock_rmtree,
        patch("src.processing.nats_msg.os.makedirs") as _,
        patch(
            "src.processing.nats_msg.asyncio.to_thread",
            side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs),  # pyrefly: ignore[implicit-any-lambda]
        ) as _,
    ):
        mock_check.return_value = False
        yield {
            "check": mock_check,
            "update_stage": mock_update_stage,
            "update_failed": mock_update_failed,
            "fetch": mock_fetch,
            "select": mock_select,
            "upscale": mock_upscale,
            "downscale": mock_downscale,
            "upload": mock_upload,
            "pub": mock_pub,
            "rmtree": mock_rmtree,
        }
