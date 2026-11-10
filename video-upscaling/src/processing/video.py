from threading import Event
from typing import Optional, Callable
from pathlib import Path
from queue import Queue
from subprocess import CalledProcessError, DEVNULL, PIPE, Popen, TimeoutExpired

from core import settings, JobCancelledError, output_dir
from .batch import flush_and_upscale_batch
from .ffmpeg import kill_on_deadline, _probe_duration_s, _parse_out_time_s, _video_encoder, _video_decoder, _extract_video_info
from .upscale_ml_model import _load_model, Resolution
from .worker import encoder_worker

import threading
import numpy as np


def video_upscale(
    cancel_event: Event,
    job_id: str,
    video_path: str, 
    model_path: Path, 
    scale: int,
    target_res: str,
    on_progress: Callable[[int], None] | None = None
) -> None:
    """Upscale a video using the mode and writes audio-less result to temp dir"""
    w, h, fps, total_frames = _extract_video_info(video_path)
    out_w, out_h = w * scale, h * scale

    per_frame_s = {2: settings.UPSCALE_2X_TIMEOUT_S_PER_FRAME, 4: settings.UPSCALE_4X_TIMEOUT_S_PER_FRAME}
    if scale not in per_frame_s:
        raise ValueError(f"unsupported scale {scale}")
    timeout_s = settings.FFMPEG_TIMEOUT_GRACE_S + total_frames * per_frame_s[scale]

    upsampler = _load_model(model_path, scale)

    decoder = _video_decoder(video_path)
    encoder = _video_encoder(fps, out_w, out_h, f"{output_dir(job_id)}/upscaled_noaudio.mp4", target_res)

    encode_queue: Queue[Optional[np.ndarray]] = Queue(maxsize=4)

    encoder_fail_event = Event()
    encoder_thread = threading.Thread(
        target=encoder_worker, args=(encode_queue, encoder, encoder_fail_event), daemon=True
    )
    encoder_thread.start()

    def raise_if_interrupted() -> None:
        if cancel_event.is_set():
            raise JobCancelledError(f"video_upscale cancelled for job {job_id}")
        if timed_out.is_set():
            raise TimeoutExpired("video upscale", timeout_s)
        if encoder_fail_event.is_set():
            raise Exception("encoder failed")
        
    frame_bytes = h * w * 3
    n_frames = 0
    pending: list[np.ndarray] = []

    with kill_on_deadline([decoder, encoder], timeout_s, cancel_event) as timed_out:
        try:
            while True:
                # check for cancel and stop and cleanup before running any processing
                raise_if_interrupted()
                if not decoder.stdout:
                    break

                raw = decoder.stdout.read(frame_bytes)
                if len(raw) < frame_bytes:
                    break

                rgb = np.frombuffer(raw, dtype=np.uint8).reshape(h, w, 3).copy()
                pending.append(rgb)

                if len(pending) == settings.BATCH_SIZE:
                    flush_and_upscale_batch(upsampler, pending, encode_queue)
                    n_frames += len(pending)

                    if on_progress is not None:
                        pct = min(99, int(n_frames / total_frames * 100))
                        on_progress(pct)

                    pending.clear()

            raise_if_interrupted()

            if pending:
                flush_and_upscale_batch(upsampler, pending, encode_queue)
                n_frames += len(pending)
        except BaseException:
            # kill so the encoder thread's join doesn't wait on ffmpeg finalizing a partial file
            encoder.kill()
            raise
        finally:
            encode_queue.put(None)
            encoder_thread.join()

        raise_if_interrupted()


def video_downscale(
    cancel_event: Event, 
    video_path: str, 
    target_res: str, 
    output_path: str, 
    on_progress: Callable[[int], None] | None = None
) -> None:
    """
    Uses ffmpeg to downscale a video to a lower res. Used when the target resolution
    is the same as the source resolution or less than the source resolution

    Usage:
        decoder.stdout.read(frame_bytes)

    Args:
        cancel_event: event thats set to tell downscale to kill ffmpeg and stop downscaling
        video_path: path to where the video is fetched and downloaded to from seaweedfs storage
        target_res: the resolution to downscale to
        output_path: path to where the final downscaled video is saved to
        on_progress: callback invoked with 0-99 as ffmpeg reports progress
    
    Raises:
        RuntimeError when calling the ffmpeg subprocess fails with an error
    """
    try:
        if cancel_event.is_set():
            raise JobCancelledError("video_upscale cancelled for downscale")
        
        tgt_res = Resolution.from_string(target_res)
        duration_s = _probe_duration_s(video_path)
        timeout_s = settings.FFMPEG_TIMEOUT_GRACE_S + duration_s * settings.DOWNSCALE_TIMEOUT_FACTOR
        proc = Popen(
            [
                "ffmpeg",
                "-protocol_whitelist", "file,pipe", "-i", video_path,
                "-vf", f"scale=-2:{tgt_res}",
                "-c:a", "copy",
                "-progress", "pipe:1",
                "-nostats",
                output_path
            ],
            stdout=PIPE, stderr=DEVNULL, text=True
        )

        with kill_on_deadline([proc], timeout_s, cancel_event) as timed_out:
            for line in proc.stdout or []:
                if cancel_event.is_set():
                    raise JobCancelledError("video_upscale cancelled for downscale")
                if not line.startswith("out_time=") or on_progress is None:
                    continue

                out_time_s = _parse_out_time_s(line.strip())
                if out_time_s is not None:
                    on_progress(min(99, int(out_time_s / duration_s * 100)))

            if cancel_event.is_set():
                raise JobCancelledError("video_upscale cancelled for downscale")
            if timed_out.is_set():
                raise TimeoutExpired("ffmpeg downscale", timeout_s)
            if proc.wait() != 0:
                raise CalledProcessError(proc.returncode, proc.args)

    except CalledProcessError as e:
        raise RuntimeError(f"ffmpeg downscale failed: {e}") from e


def recombine_video_audio(
    job_id: str,
    video_path: str,
    output_path: str,
    on_progress: Optional[Callable[[int], None]] = None,
) -> None:
    """
    Use ffmpeg to recombine the no audio upscaled video with the original audio

    Args:
        job_id: the id to identify the temp video with no audio to recombine audio on
        video_path: path to the original video with audio
        output_path: the path to save the combined video to
        on_progress: callback invoked with 0-99 as ffmpeg reports progress
    """
    noaudio_path = f"{output_dir(job_id)}/upscaled_noaudio.mp4"
    duration_s = _probe_duration_s(noaudio_path)
    timeout_s = settings.FFMPEG_TIMEOUT_GRACE_S + duration_s * settings.RECOMBINE_TIMEOUT_FACTOR
    proc = Popen(
        [
            "ffmpeg", "-y",
            "-protocol_whitelist", "file,pipe", "-i", noaudio_path,
            "-protocol_whitelist", "file,pipe", "-i", video_path,
            "-map", "0:v", "-map", "1:a?",
            "-c", "copy",
            "-progress", "pipe:1", "-nostats", output_path,
        ],
        stdout=PIPE, stderr=DEVNULL, text=True
    )

    with kill_on_deadline([proc], timeout_s) as timed_out:
        for line in proc.stdout or []:
            if not line.startswith("out_time=") or on_progress is None:
                continue

            out_time_s = _parse_out_time_s(line.strip())
            if out_time_s is not None:
                on_progress(min(99, int(out_time_s / duration_s * 100)))
        if timed_out.is_set():
            raise TimeoutExpired("ffmpeg recombine", timeout_s)
        if proc.wait() != 0:
            raise CalledProcessError(proc.returncode, proc.args)