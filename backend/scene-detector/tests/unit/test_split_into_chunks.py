from unittest.mock import patch
from unittest.mock import MagicMock
from src.processing.video import split_into_chunks
import os
import tempfile


def test_returns_correct_chunk_paths() -> None:
    """Returns zero-padded scene paths based on detected scene count"""
    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch(
                "src.processing.video.detect",
                return_value=[MagicMock(), MagicMock(), MagicMock],
            ),
            patch("src.processing.video.split_video_ffmpeg"),
        ):
            result = split_into_chunks("/videos/myvideo.mp4", output_dir)

    assert result == [
        os.path.join(output_dir, "myvideo-Scene-001.mp4"),
        os.path.join(output_dir, "myvideo-Scene-002.mp4"),
        os.path.join(output_dir, "myvideo-Scene-003.mp4"),
    ]


def test_returns_empty_list_when_no_scenes() -> None:
    """Returns empty list when no scenes are detected"""
    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch("src.processing.video.detect", return_value=[]),
            patch("src.processing.video.split_video_ffmpeg"),
        ):
            result = split_into_chunks("/explosion/myvideo.mp4", output_dir)

    assert result == []
