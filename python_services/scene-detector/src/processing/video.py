import glob
import os
import shutil
import subprocess
from threading import Event
from typing import Callable, Optional

from scenedetect import (
    AdaptiveDetector,
    FrameTimecode,
    SceneManager,
    open_video,
)
from scenedetect.video_splitter import DEFAULT_FFMPEG_ARGS
from shared_handler.exceptions import JobCancelledError

DETECT_SLICE_FRAMES = 150  # frames processed per detect_scenes() call


def split_into_chunks(
    cancel_event: Event,
    video_path: str,
    output_dir: str,
    on_progress: Optional[Callable[[int], None]] = None,
) -> list[str]:
    """
    Split one video file into multiple video chunks based on scene
    change.

    Args:
        cancel_event: this is set when cancel.{job_id} nats msg is published to signal to stop processing
        video_path: the location the video we are trying to split is
        output_dir: the location the split video is saved to
        on_progress: optional callback invoked with 0-100 as work proceeds — 0-90 during
            the detection scan, 90-100 while splitting scenes into output chunks

    Returns:
        a list of output video dir strings

    Raises:
        JobCancelledError: when the cancel event is set and stops processing
    """

    video = open_video(video_path)
    scene_manager = SceneManager()
    scene_manager.add_detector(AdaptiveDetector())

    total_frames = video.duration.frame_num if video.duration else None
    while (
        scene_manager.detect_scenes(
            video=video, duration=FrameTimecode(DETECT_SLICE_FRAMES, video.frame_rate)
        )
        > 0
    ):
        if cancel_event.is_set():
            raise JobCancelledError("split_into_chunks cancelled during detect scan")

        if on_progress and total_frames:
            on_progress(min(90, int(video.frame_number / total_frames * 90)))

    if cancel_event.is_set():
        raise JobCancelledError("split_into_chunks cancelled after detect scan")

    scene_list = scene_manager.get_scene_list()

    if len(scene_list) <= 1:
        os.makedirs(output_dir, exist_ok=True)
        dest = os.path.join(output_dir, os.path.basename(video_path))
        shutil.copy2(video_path, dest)
        if on_progress:
            on_progress(100)
        return [dest]

    os.makedirs(output_dir, exist_ok=True)
    video_stem = os.path.splitext(os.path.basename(video_path))[0]

    # N scenes need N-1 split points, scene 1 starts at 0. The same string feeds both flags
    # so the forced keyframes and the segment cuts can never disagree
    cuts = ",".join(str(start.get_seconds()) for start, _ in scene_list[1:])
    total_us = int(video.duration.get_seconds() * 1_000_000) if video.duration else None

    proc = subprocess.Popen(
        [
            "ffmpeg",
            "-nostdin",
            "-y",
            "-progress",
            "pipe:1",
            "-i",
            video_path,
            *DEFAULT_FFMPEG_ARGS.split(" "),
            "-sn",
            "-force_key_frames",
            cuts,
            "-f",
            "segment",
            "-segment_times",
            cuts,
            "-segment_start_number",
            "1",
            "-reset_timestamps",
            "1",
            os.path.join(output_dir, f"{video_stem}-Scene-%03d.mp4"),
        ],
        stdout=subprocess.PIPE,
        text=True,
    )
    try:
        for line in proc.stdout:
            if cancel_event.is_set():
                proc.terminate()
                raise JobCancelledError(
                    "split_into_chunks cancelled during scene-split"
                )
            if total_us and on_progress and line.startswith("out_time_us="):
                value = line.split("=")[1].strip()
                if value.isdigit():
                    on_progress(90 + int(min(int(value) / total_us, 1) * 10))
        if proc.wait() != 0:
            raise subprocess.CalledProcessError(proc.returncode, proc.args)
    finally:
        if proc.poll() is None:
            proc.kill()

    output_paths = sorted(
        glob.glob(os.path.join(output_dir, f"{video_stem}-Scene-*.mp4"))
    )
    if len(output_paths) != len(scene_list):
        raise RuntimeError(
            f"expected {len(scene_list)} scene chunks but ffmpeg produced {len(output_paths)}"
        )

    if on_progress:
        on_progress(100)

    return output_paths
