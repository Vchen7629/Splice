from scenedetect import detect
from scenedetect import AdaptiveDetector
from scenedetect import split_video_ffmpeg
from pathlib import Path
import os
import shutil


def split_into_chunks(video_path: str, output_dir: str) -> list[str]:
    """
    Split one video file into multiple video chunks based on scene
    Change.

    Args:
        video_path: the location the video we are trying to split is
        output_dir: the location the split video is saved to

    Returns:
        a list of output video dir strings
    """
    scene_list = detect(video_path, AdaptiveDetector())

    if not scene_list:
        os.makedirs(output_dir, exist_ok=True)
        dest = os.path.join(output_dir, os.path.basename(video_path))
        shutil.copy2(video_path, dest)
        return [dest]

    split_video_ffmpeg(video_path, scene_list, Path(output_dir))
    video_stem = os.path.splitext(os.path.basename(video_path))[0]

    return [
        os.path.join(output_dir, f"{video_stem}-Scene-{i + 1:03d}.mp4")
        for i in range(len(scene_list))
    ]
