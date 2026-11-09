from contextlib import contextmanager
from subprocess import DEVNULL, PIPE, Popen, run
from threading import Event, Thread
from time import monotonic
from typing import Generator, Optional, Sequence

from torch.cuda import is_available

from core import settings

from .upscale_ml_model import Resolution


@contextmanager
def kill_on_deadline(
    procs: Sequence[Popen[str] | Popen[bytes]],
    timeout_s: float,
    cancel_event: Event | None = None,
) -> Generator[Event, None, None]:
    """Terminates proc if timeout_s elapses or cancel_event is set"""
    watcher_stop, timed_out = Event(), Event()
    deadline = monotonic() + timeout_s

    def kill_all() -> None:
        for proc in procs:
            if proc.poll() is None:
                proc.kill()

    def watch_and_kill() -> None:
        while not watcher_stop.wait(0.1):
            if cancel_event is not None and cancel_event.is_set():
                kill_all()
                return
            if monotonic() > deadline:
                timed_out.set()
                kill_all()
                return

    watcher = Thread(target=watch_and_kill, daemon=True)
    watcher.start()
    try:
        yield timed_out
    finally:
        watcher_stop.set()
        watcher.join()
        kill_all()
        for proc in procs:
            proc.wait()
            if proc.stdout:
                proc.stdout.close()


def _probe_duration_s(video_path: str) -> float:
    """use ffprobe to extract the video duration in seconds"""
    stream_duration = _run_ffprobe(
        video_path, "-select_streams", "v:0", "-show_entries", "stream=duration"
    )
    if stream_duration != "N/A":
        return float(stream_duration)

    format_duration = _run_ffprobe(video_path, "-show_entries", "format=duration")
    if format_duration != "N/A":
        return float(format_duration)

    _, _, fps, total_frames = _extract_video_info(video_path)
    return total_frames / fps


def _extract_video_info(video_path: str) -> tuple[int, int, float, int]:
    """use ffprobe to extract video information like w, h, and fps of a video"""
    ffprobe = _run_ffprobe(
        video_path,
        "-select_streams",
        "v:0",
        "-show_entries",
        "stream=width,height,r_frame_rate,nb_frames",
    )

    w, h, fps_frac, nb_frames = ffprobe.split(",")

    fps_num, fps_den = fps_frac.split("/")
    if float(fps_den) == 0:
        raise RuntimeError(f"invalid frame rate {fps_frac!r}")
    fps = float(fps_num) / float(fps_den)

    if nb_frames == "N/A":
        # some containers (e.g. webm from MediaRecorder) don't store a frame count
        # in the header. count packets (demux only, no decode) rather than estimating
        # from duration * r_frame_rate, which is off for variable frame rate video
        nb_frames = _run_ffprobe(
            video_path,
            "-select_streams",
            "v:0",
            "-count_packets",
            "-show_entries",
            "stream=nb_read_packets",
        )

    return int(w), int(h), fps, int(nb_frames)


def _parse_out_time_s(line: str) -> Optional[float]:
    """Parse an `out_time=HH:MM:SS.ms` line from ffmpeg's -progress output into seconds"""
    value = line.split("=", 1)[1]
    if value == "N/A":
        return None
    h, m, s = value.split(":")
    return int(h) * 3600 + int(m) * 60 + float(s)


# maps ffprobe codec_name to matching NVDEC (cuvid) decoder
_CUVID_DECODERS = {
    "h264": "h264_cuvid",
    "hevc": "hevc_cuvid",
    "vp8": "vp8_cuvid",
    "vp9": "vp9_cuvid",
    "mpeg2video": "mpeg2_cuvid",
    "mpeg4": "mpeg4_cuvid",
    "av1": "av1_cuvid",
}


def _video_decoder(video_path: str) -> Popen[bytes]:
    """reads the input video file and outputs raw pixel data (RGB24) frame-by-frame to stdout.
    Used to pipe directly into the model to upscale frames without writing temp png files
    """
    hwaccel_args: list[str] = []
    if is_available():  # check if cuda is on machine
        codec = _run_ffprobe(
            video_path, "-select_streams", "v:0", "-show_entries", "stream=codec_name"
        )
        cuvid_decoder = _CUVID_DECODERS.get(codec)
        if cuvid_decoder is not None:
            hwaccel_args = ["-hwaccel", "cuda", "-c:v", cuvid_decoder]

    return Popen(
        [
            "ffmpeg",
            *hwaccel_args,
            "-protocol_whitelist",
            "file,pipe",
            "-i",
            video_path,
            "-f",
            "rawvideo",
            "-pix_fmt",
            "rgb24",
            "-",
        ],
        stdout=PIPE,
        stderr=DEVNULL,
    )


def _run_ffprobe(video_path: str, *extra_args: str) -> str:
    """helper to run ffprobe to extract video info like duration, codec etc"""
    if not video_path:
        raise TypeError("Missing video_path input")

    probe = run(
        [
            "ffprobe",
            "-v",
            "error",
            *extra_args,
            "-of",
            "csv=p=0",
            "-protocol_whitelist",
            "file,pipe",
            video_path,
        ],
        capture_output=True,
        text=True,
        check=True,
        timeout=settings.FFPROBE_TIMEOUT_S,
    )

    return probe.stdout.strip()


def _video_encoder(
    fps: float, out_w: int, out_h: int, out_path: str, target_res: str
) -> Popen[bytes]:
    """
    Long running encoder that takes in upscaled frames from encode_worker via stdin
    and encodes the frames to the correct resolution and framerate as a compressed
    H264 .mp4 video file with no audio
    """
    if fps <= 0:
        raise ValueError("fps cant be negative or 0")
    if out_w <= 0:
        raise ValueError("out_w cant be negative or 0")
    if out_h is None or out_h <= 0:
        raise ValueError("out_h cant be negative or 0")

    vf = ["-vf", f"scale=-2:{Resolution.from_string(target_res)}"]
    return Popen(
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
            "-protocol_whitelist",
            "file,pipe",
            "-i",
            "pipe:0",
            *vf,
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
        stdin=PIPE,
        stderr=DEVNULL,
    )
