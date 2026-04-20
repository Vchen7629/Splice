from subprocess import Popen
import subprocess


def extract_video_info(video_path: str) -> tuple[int, int, float]:
    """
    use ffprobe to extract video information like w, h, and fps of a video

    Args:
        video_path: the path to the video we are trying to process

    Returns:
        a tuple containing the width, height, and fps of the video

    Raises:
        TypeError if the video_path is not provided
    """
    if not video_path:
        raise TypeError("Missing video_path input")

    probe = subprocess.run(
        [
            "ffprobe",
            "-v",
            "error",
            "-select_streams",
            "v:0",
            "-show_entries",
            "stream=width,height,r_frame_rate",
            "-of",
            "csv=p=0",
            video_path,
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    w, h, fps_frac = probe.stdout.strip().split(",")

    w, h = int(w), int(h)

    fps_num, fps_den = fps_frac.split("/")
    fps = float(fps_num) / float(fps_den)

    return w, h, fps


def recombine_video_audio(video_path: str, output_path: str) -> None:
    """
    Use ffmpeg to recombine the no audio upscaled video with the original audio

    Args:
        video_path: path to the original video with audio
        output_path: the path to save the combined video to
    """
    subprocess.run(
        [
            "ffmpeg",
            "-y",
            "-i",
            "/tmp/upscaled_noaudio.mp4",
            "-i",
            video_path,
            "-map",
            "0:v",
            "-map",
            "1:a?",
            "-c",
            "copy",
            output_path,
        ],
        check=True,
        stderr=subprocess.DEVNULL,
    )


def video_decoder(video_path: str) -> Popen[bytes]:
    """
    Uses ffmpeg to read the input video file to output raw pixel
    data (RGB24) frame-by-frame to stdout. Used to pipe directly into
    the model to upscale frames without writing temp png files

    Usage:
        decoder.stdout.read(frame_bytes)

    Args:
        video_path: path to video to process

    Returns:
        decoder instance
    """
    # todo: detect cuda and switch between cpu and hwaccel
    return subprocess.Popen(
        [
            "ffmpeg",
            "-hwaccel",
            "cuda",
            "-c:v",
            "h264_cuvid",
            "-i",
            video_path,
            "-f",
            "rawvideo",
            "-pix_fmt",
            "rgb24",
            "-",
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )


def video_encoder(fps: float, out_w: int, out_h: int, out_path: str) -> Popen[bytes]:
    """
    Long running encoder that takes in upscaled frames from encode_worker via stdin
    and encodes the frames to the correct resolution and framerate as a compressed
    H264 .mp4 video file with no audio

    Args:
        fps: the desired fps for the compressed video file
        out_w: the desired video width for the compressed video file
        out_h: the desired video height for the compressed video file
        out_path: the path for the compressed video file to be saved to

    Raises:
        ValueError if fps, out_w, or out_h is invalid (negative value)
    """
    if fps <= 0:
        raise ValueError("fps cant be negative or 0")
    if out_w <= 0:
        raise ValueError("out_w cant be negative or 0")
    if out_h is None or out_h <= 0:
        raise ValueError("out_h cant be negative or 0")

    return subprocess.Popen(
        [
            "ffmpeg",
            "-y",
            "-f",
            "rawvideo",
            "-pix_fmt",
            "yuv420p",
            "-s",
            f"{out_w}x{out_h}",
            "-r",
            str(fps),
            "-i",
            "pipe:0",
            "-c:v",
            "libx264",
            "-crf",
            "18",
            "-preset",
            "ultrafast",
            "-pix_fmt",
            "yuv420p",
            out_path,
        ],
        stdin=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
