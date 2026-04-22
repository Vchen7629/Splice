from typing import Optional
from pathlib import Path
from queue import Queue
from subprocess import Popen
from utils.metrics import log_timing
from utils.model_router import Resolution
from core.settings import settings
from processing.batch import flush_batch
from processing.worker import encode_worker
from processing.load_model import load_model
import time
import threading
import subprocess
import numpy as np

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

    probe = subprocess.run([
        "ffprobe", "-v", "error",
        "-select_streams", "v:0",
        "-show_entries", "stream=width,height,r_frame_rate",
        "-of", "csv=p=0",
        video_path
    ], capture_output=True, text=True, check=True)
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
    subprocess.run([
        "ffmpeg", "-y",
        "-i", "/tmp/upscaled_noaudio.mp4",
        "-i", video_path,
        "-map", "0:v", "-map", "1:a?",
        "-c", "copy",
        output_path
    ], check=True, stderr=subprocess.DEVNULL)

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
    return subprocess.Popen([
        "ffmpeg", "-hwaccel", "cuda", "-c:v", "h264_cuvid",
        "-i", video_path,
        "-f", "rawvideo", "-pix_fmt", "rgb24", "-"
    ], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)

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

def video_downscale(video_path: str, target_res: str, output_path: str) -> None:
    """
    Uses ffmpeg to downscale a video to a lower res. Used when the target resolution
    is the same as the source resolution or less than the source resolution

    Usage:
        decoder.stdout.read(frame_bytes)

    Args:
        video_path: path to where the video is fetched and downloaded to from seaweedfs storage
        target_res: the resolution to downscale to
        output_path: path to where the final downscaled video is saved to
    
    Raises:
        RuntimeError when calling the ffmpeg subprocess fails with an error
    """
    try:
        tgt_res = Resolution.from_string(target_res)
        
        subprocess.run([
            "ffmpeg",
            "-i", video_path,
            "-vf", f"scale=-2:{tgt_res}",
            "-c:a", "copy",
            output_path
        ], check=True)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"ffmpeg downscale failed: {e.stderr.decode()}") from e


def video_upscale(video_path: str, output_path: str, model_path: Path, scale: int) -> None:
    w, h, fps = extract_video_info(video_path)

    out_w, out_h = w * scale, h * scale

    upsampler = load_model(model_path, scale)

    decoder = video_decoder(video_path)
    encoder = video_encoder(fps, out_w, out_h, "/tmp/upscaled_noaudio.mp4")

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

    recombine_video_audio(video_path, output_path)
