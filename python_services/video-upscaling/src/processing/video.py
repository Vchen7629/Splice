from threading import Event
from typing import Optional, Callable
from pathlib import Path
from queue import Queue
from subprocess import Popen
from utils import log_timing, Resolution
from shared_handler import JobCancelledError
from core.settings import settings
from .batch import flush_batch
from .worker import encode_worker
from .load_model import load_model
import time
import threading
import subprocess
import numpy as np
import torch

def extract_video_info(video_path: str) -> tuple[int, int, float, int]:
    """
    use ffprobe to extract video information like w, h, and fps of a video

    Args:
        video_path: the path to the video we are trying to process

    Returns:
        a tuple containing the width, height, fps, and num frames of the video

    Raises:
        TypeError if the video_path is not provided
    """
    if not video_path:
        raise TypeError("Missing video_path input")

    probe = subprocess.run([
        "ffprobe", "-v", "error",
        "-select_streams", "v:0",
        "-show_entries", "stream=width,height,r_frame_rate,nb_frames",
        "-of", "csv=p=0",
        video_path
    ], capture_output=True, text=True, check=True)

    w, h, fps_frac, nb_frames = probe.stdout.strip().split(",")

    fps_num, fps_den = fps_frac.split("/")
    fps = float(fps_num) / float(fps_den)

    if nb_frames == "N/A":
        # some containers (e.g. webm from MediaRecorder) don't store a frame
        # count or duration in the header, so it has to be counted by decoding
        count_probe = subprocess.run([
            "ffprobe", "-v", "error",
            "-select_streams", "v:0",
            "-count_frames",
            "-show_entries", "stream=nb_read_frames",
            "-of", "csv=p=0",
            video_path
        ], capture_output=True, text=True, check=True)
        nb_frames = count_probe.stdout.strip()

    return int(w), int(h), fps, int(nb_frames)


def recombine_video_audio(
    job_id: str,
    video_path: str,
    output_path: str,
    target_res: str | None = None,
    on_progress: Optional[Callable[[int], None]] = None,
) -> None:
    """
    Use ffmpeg to recombine the no audio upscaled video with the original audio

    Args:
        job_id: the id to identify the temp video with no audio to recombine audio on
        video_path: path to the original video with audio
        output_path: the path to save the combined video to
        target_res: if given scales the video to exact resolution
        on_progress: callback invoked with 0-99 as ffmpeg reports progress
    """
    noaudio_path = f"/tmp/upscaled_noaudio-{job_id}.mp4"
    cmd = [
        "ffmpeg", "-y",
        "-i", noaudio_path,
        "-i", video_path,
        "-map", "0:v", "-map", "1:a?",
    ]

    if target_res is not None:
        tgt_res = Resolution.from_string(target_res)
        cmd += ["-vf", f"scale=-2:{tgt_res}", "-c:v", "libx264", "-crf", "18", "-c:a", "copy"]
    else:
        cmd += ["-c", "copy"]

    cmd += ["-progress", "pipe:1", "-nostats", output_path]

    duration_s = _probe_duration_s(noaudio_path)
    proc = subprocess.Popen(
        cmd, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True
    )

    for line in proc.stdout or []:
        if not line.startswith("out_time=") or on_progress is None:
            continue

        out_time_s = _parse_out_time_s(line.strip())
        if out_time_s is not None:
            on_progress(min(99, int(out_time_s / duration_s * 100)))

    if proc.wait() != 0:
        raise subprocess.CalledProcessError(proc.returncode, cmd)


def _probe_duration_s(video_path: str) -> float:
    """Use ffprobe to get a video's duration in seconds, falling back to frame-count/fps for 
    containers (e.g. webm from MediaRecorder) that don't store a duration in their header"""
    probe = subprocess.run([
        "ffprobe", "-v", "error",
        "-show_entries", "format=duration",
        "-of", "csv=p=0",
        video_path
    ], capture_output=True, text=True, check=True)

    duration = probe.stdout.strip()
    if duration != "N/A":
        return float(duration)

    _, _, fps, total_frames = extract_video_info(video_path)
    return total_frames / fps


def _parse_out_time_s(line: str) -> Optional[float]:
    """Parse an `out_time=HH:MM:SS.ms` line from ffmpeg's -progress output into seconds"""
    value = line.split("=", 1)[1]
    if value == "N/A":
        return None
    h, m, s = value.split(":")
    return int(h) * 3600 + int(m) * 60 + float(s)


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
    hwaccel_args: list[str] = []
    if torch.cuda.is_available():
        codec = _probe_video_codec(video_path)
        cuvid_decoder = _CUVID_DECODERS.get(codec)
        if cuvid_decoder is not None:
            hwaccel_args = ["-hwaccel", "cuda", "-c:v", cuvid_decoder]

    return subprocess.Popen([
        "ffmpeg", *hwaccel_args,
        "-i", video_path,
        "-f", "rawvideo", "-pix_fmt", "rgb24", "-"
    ], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)

# maps ffprobe codec_name to matching NVDEC (cuvid) decoder
_CUVID_DECODERS = {
    "h264": "h264_cuvid",
    "hevc": "hevc_cuvid",
    "vp8": "vp8_cuvid",
    "vp9": "vp9_cuvid",
    "mpeg2video": "mpeg2_cuvid",
    "mpeg4": "mpeg4_cuvid",
    "av1": "av1_cuvid"
}

def _probe_video_codec(video_path: str) -> str:
    """Use ffprobe to get the video stream's codec name"""
    probe = subprocess.run([
        "ffprobe", "-v", "error",
        "-select_streams", "v:0",
        "-show_entries", "stream=codec_name",
        "-of", "csv=p=0",
        video_path
    ], capture_output=True, text=True, check=True)

    return probe.stdout.strip()

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

    return subprocess.Popen([
        "ffmpeg", "-y",
        "-f", "rawvideo", "-pix_fmt", "yuv420p",
        "-s", f"{out_w}x{out_h}",
        "-r", str(fps),
        "-i", "pipe:0",
        "-c:v", "libx264", "-crf", "18",
        "-preset", "ultrafast",
        "-pix_fmt", "yuv420p",
        out_path
    ], stdin=subprocess.PIPE, stderr=subprocess.DEVNULL)

def video_downscale(
    video_path: str, target_res: str, output_path: str, on_progress: Callable[[int], None] | None = None
) -> None:
    """
    Uses ffmpeg to downscale a video to a lower res. Used when the target resolution
    is the same as the source resolution or less than the source resolution

    Usage:
        decoder.stdout.read(frame_bytes)

    Args:
        video_path: path to where the video is fetched and downloaded to from seaweedfs storage
        target_res: the resolution to downscale to
        output_path: path to where the final downscaled video is saved to
        on_progress: callback invoked with 0-99 as ffmpeg reports progress
    
    Raises:
        RuntimeError when calling the ffmpeg subprocess fails with an error
    """
    try:
        tgt_res = Resolution.from_string(target_res)
        cmd = [
            "ffmpeg",
            "-i", video_path,
            "-vf", f"scale=-2:{tgt_res}",
            "-c:a", "copy",
            "-progress", "pipe:1",
            "-nostats",
            output_path
        ]

        duration_s = _probe_duration_s(video_path)
        proc = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True
        )

        for line in proc.stdout or []:
            if not line.startswith("out_time=") or on_progress is None:
                continue

            out_time_s = _parse_out_time_s(line.strip())
            if out_time_s is not None:
                on_progress(min(99, int(out_time_s / duration_s * 100)))

        if proc.wait() != 0:
            raise subprocess.CalledProcessError(proc.returncode, cmd)

    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"ffmpeg downscale failed: {e}") from e

def video_upscale(
    cancel_event: Event,
    job_id: str,
    video_path: str, 
    model_path: Path, 
    scale: int, 
    on_progress: Callable[[int], None] | None = None
) -> None:
    """Upscale a video using the model, writing audio-less result to /tmp/upscaled_noaudio.mp4"""
    w, h, fps, total_frames = extract_video_info(video_path)

    out_w, out_h = w * scale, h * scale

    upsampler = load_model(model_path, scale)

    decoder = video_decoder(video_path)
    encoder = video_encoder(fps, out_w, out_h, f"/tmp/upscaled_noaudio-{job_id}.mp4")

    encode_queue: Queue[Optional[bytes]] = Queue(maxsize=4)

    encode_thread = threading.Thread(
        target=encode_worker, args=(encode_queue, encoder), daemon=True
    )
    encode_thread.start()

    frame_bytes = h * w * 3
    t_read = t_infer = t_enq = 0.0
    n_frames = 0
    n_batches = 0
    pending: list[np.ndarray] = []

    while True:
        # check for cancel and stop and cleanup before running any processing
        if cancel_event.is_set():
            if decoder.stdout:
                decoder.stdout.close()
            decoder.kill()
            encoder.kill()
            encode_queue.put(None)
            encode_thread.join()
            raise JobCancelledError(f"video_upscale cancelled for job {job_id}")

        t0 = time.perf_counter()
        if not decoder.stdout:
            break

        raw = decoder.stdout.read(frame_bytes)
        t_read += time.perf_counter() - t0
        if len(raw) < frame_bytes:
            break

        bgr = np.frombuffer(raw, dtype=np.uint8).reshape(h, w, 3)[:, :, ::-1].copy()
        pending.append(bgr)

        if len(pending) == settings.BATCH_SIZE:
            dt_infer, dt_enq, n = flush_batch(upsampler, pending, encode_queue)
            t_infer += dt_infer
            t_enq += dt_enq
            n_frames += n
            n_batches += 1

            if on_progress is not None:
                pct = min(99, int(n_frames / total_frames * 100))
                on_progress(pct)

            pending.clear()

    if pending:
        dt_infer, dt_enq, n = flush_batch(upsampler, pending, encode_queue)
        t_infer += dt_infer
        t_enq += dt_enq
        n_frames += n
        n_batches += 1

    if decoder.stdout:
        decoder.stdout.close()
    decoder.wait()
    encode_queue.put(None)

    t_enc_start = time.perf_counter()
    encode_thread.join()
    t_enc = time.perf_counter() - t_enc_start

    log_timing(t_read, t_infer, t_enq, t_enc, n_frames, n_batches)
