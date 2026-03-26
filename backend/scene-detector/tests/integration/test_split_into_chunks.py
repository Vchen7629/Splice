from src.processing.video import split_into_chunks
import os
import tempfile

VIDEO_PATH = os.path.join(os.path.dirname(__file__), "../videos/ForBiggerBlazes.mp4")

def test_splits_video_and_returns_existing_paths() -> None:
    with tempfile.TemporaryDirectory() as output_dir:
        chunk_paths = split_into_chunks(VIDEO_PATH, output_dir)

        assert len(chunk_paths) > 0
        for path in chunk_paths:
            assert os.path.exists(path), f"expected chunk not found: {path}"