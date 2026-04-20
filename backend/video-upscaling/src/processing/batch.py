from queue import Queue
from typing import Any
from realesrgan import RealESRGANer
from time import perf_counter
import torch
import numpy as np


def flush_batch(
    upsampler: RealESRGANer, frames: list[Any], encode_queue: Queue
) -> tuple[float, float, int]:
    """
    Runs one batch of frames through GPU model and queues the results for encoding
    1. Calls infer_batch to send the batch of raw frames through the upscaler model
    2. puts each result in encoder_queue for encoder_worker to write to ffmpeg

    Args:
        upsampler: the upscaling model
        frames: list containing the batch of unupscaled video frames to process
        encode_queue: the queue that upscaled images are written to

    Returns:
        a tuple containing timing metrics and frame count for processing stats
    """
    t0 = perf_counter()
    results = _infer_batch(upsampler.model, frames)

    t1 = perf_counter()
    for r in results:
        encode_queue.put(r)

    t2 = perf_counter()

    return t1 - t0, t2 - t1, len(frames)


def _infer_batch(model: RealESRGANer, frames_bgr: list) -> list[Any]:
    """
    Upscales a batch of BGR frames. Converts to YUV420p on GPU before CPU transfer
    to minimize PCIe bandwidth and maximize calculations on gpu for speed
    """
    tensors = [
        torch.from_numpy(f[:, :, ::-1].copy()).permute(2, 0, 1).half() / 255.0
        for f in frames_bgr
    ]
    batch = torch.stack(tensors).cuda()  # (N, 3, H, W) fp16

    with torch.no_grad():
        with torch.autocast(device_type="cuda"):
            out = model(batch)  # (N, 3, out_H, out_W)

    # convert to yuv420p on gpu before dtoh to reduce pipe data requirements
    R, G, B = out[:, 0], out[:, 1], out[:, 2]
    Y = (16 + 65.481 * R + 128.553 * G + 24.966 * B).clamp(16, 235).byte().cpu().numpy()
    U = (
        (
            128
            - 37.797 * R[:, ::2, ::2]
            - 74.203 * G[:, ::2, ::2]
            + 112.0 * B[:, ::2, ::2]
        )
        .clamp(16, 240)
        .byte()
        .cpu()
        .numpy()
    )
    V = (
        (
            128
            + 112.0 * R[:, ::2, ::2]
            - 93.786 * G[:, ::2, ::2]
            - 18.214 * B[:, ::2, ::2]
        )
        .clamp(16, 240)
        .byte()
        .cpu()
        .numpy()
    )

    return [
        np.concatenate([Y[i].ravel(), U[i].ravel(), V[i].ravel()]).tobytes()
        for i in range(len(frames_bgr))
    ]
