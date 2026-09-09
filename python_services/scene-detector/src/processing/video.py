from scenedetect import (
    open_video,
    SceneManager,
    AdaptiveDetector,
    FrameTimecode,
)
from scenedetect.video_splitter import DEFAULT_FFMPEG_ARGS
from threading import Event
from typing import Callable, Optional
from shared_handler.exceptions import JobCancelledError
import os
import shutil
import subprocess

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

    if not scene_list:
        os.makedirs(output_dir, exist_ok=True)
        dest = os.path.join(output_dir, os.path.basename(video_path))
        shutil.copy2(video_path, dest)
        if on_progress:
            on_progress(100)
        return [dest]

    os.makedirs(output_dir, exist_ok=True)
    video_stem = os.path.splitext(os.path.basename(video_path))[0]
    output_paths = []

    for i, (start, end) in enumerate(scene_list):
        if cancel_event.is_set():
            raise JobCancelledError(
                f"split_into_chunks cancelled before scene {i} for job"
            )

        output_path = os.path.join(output_dir, f"{video_stem}-Scene-{i + 1:03d}.mp4")
        subprocess.run(
            [
                "ffmpeg",
                "-v",
                "quiet",
                "-nostdin",
                "-y",
                "-ss",
                str(start.get_seconds()),
                "-i",
                video_path,
                "-t",
                str((end - start).get_seconds()),
                *DEFAULT_FFMPEG_ARGS.split(" "),
                "-sn",
                output_path,
            ],
            check=True,
        )
        output_paths.append(output_path)
        if on_progress:
            on_progress(90 + int((i + 1) / len(scene_list) * 10))

    if cancel_event.is_set():
        raise JobCancelledError("split_into_chunks during scene-split")

    return output_paths
