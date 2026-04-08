from src.processing.video import split_into_chunks
import os
import subprocess
import tempfile

VIDEO_PATH = os.path.join(os.path.dirname(__file__), "../videos/ForBiggerBlazes.mp4")


def test_splits_video_and_returns_existing_paths() -> None:
    with tempfile.TemporaryDirectory() as output_dir:
        chunk_paths = split_into_chunks(VIDEO_PATH, output_dir)

        assert len(chunk_paths) > 0
        for path in chunk_paths:
            assert os.path.exists(path), f"expected chunk not found: {path}"


def test_single_scene_video_output_dir_differs_from_source_dir() -> None:
    """Single-scene video must not fail when output_dir differs from the source directory."""
    with tempfile.TemporaryDirectory() as tmp:
        src_dir = os.path.join(tmp, "source")
        out_dir = os.path.join(tmp, "chunks")
        os.makedirs(src_dir)

        video_path = os.path.join(src_dir, "single.mp4")
        subprocess.run(
            [
                "ffmpeg",
                "-y",
                "-f",
                "lavfi",
                "-i",
                "color=green:duration=2:size=320x240:rate=24",
                "-c:v",
                "libx264",
                "-pix_fmt",
                "yuv420p",
                video_path,
            ],
            check=True,
            capture_output=True,
        )

        chunk_paths = split_into_chunks(video_path, out_dir)

        assert len(chunk_paths) == 1
        assert os.path.exists(chunk_paths[0]), "output chunk not found"
        assert os.path.abspath(chunk_paths[0]) != os.path.abspath(video_path), (
            "output chunk must not overwrite the source file"
        )
