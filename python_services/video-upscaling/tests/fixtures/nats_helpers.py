from typing import Any, Generator
from unittest.mock import AsyncMock, patch

import pytest


@pytest.fixture
def nats_msg_patches() -> Generator[dict[str, Any], Any, None]:
    """Patches all external dependencies used by process_job_msg / cleanup_job."""
    with (
        patch(
            "src.processing.nats_msg.update_job_stage", new_callable=AsyncMock
        ) as mock_update_stage,
        patch(
            "src.processing.nats_msg.fetch_video", return_value="/tmp/job-123/video.mp4"
        ) as mock_fetch,
        patch("src.processing.nats_msg.select_model") as mock_select,
        patch("src.processing.nats_msg.video_upscale") as mock_upscale,
        patch("src.processing.nats_msg.video_downscale") as mock_downscale,
        patch("src.processing.nats_msg.recombine_video_audio") as mock_recombine,
        patch("src.processing.nats_msg.upload_video") as mock_upload,
        patch("src.processing.nats_msg.publisher", new_callable=AsyncMock) as mock_pub,
        patch("src.processing.nats_msg.cleanup_temp_dir") as mock_cleanup_temp_dir,
        patch("src.processing.nats_msg.cleanup_temp_file") as mock_cleanup_temp_file,
        patch("src.processing.nats_msg.os.makedirs") as _,
        patch(
            "src.processing.nats_msg.asyncio.to_thread",
            side_effect=lambda fn, *args, **kwargs: fn(*args, **kwargs),  # pyrefly: ignore[implicit-any-lambda]
        ) as _,
    ):
        yield {
            "update_stage": mock_update_stage,
            "fetch": mock_fetch,
            "select": mock_select,
            "upscale": mock_upscale,
            "downscale": mock_downscale,
            "recombine": mock_recombine,
            "upload": mock_upload,
            "pub": mock_pub,
            "cleanup_temp_dir": mock_cleanup_temp_dir,
            "cleanup_temp_file": mock_cleanup_temp_file,
        }
