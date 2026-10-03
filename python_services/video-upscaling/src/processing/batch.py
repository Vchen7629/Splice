from queue import Queue
from time import perf_counter
from typing import Optional

import numpy as np
import torch
import torch.nn.functional as F
from realesrgan import RealESRGANer


def flush_batch(
    upsampler: RealESRGANer,
    frames: list[np.ndarray],
    encode_queue: Queue[Optional[np.ndarray]],
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
    upscaled = _upscale_frames(upsampler.model, frames)
    results = _rgb_to_yuv420(upscaled)

    t1 = perf_counter()
    for r in results:
        encode_queue.put(r)

    t2 = perf_counter()

    return t1 - t0, t2 - t1, len(frames)


def _upscale_frames(
    model: torch.nn.Module, frames_rgb: list[np.ndarray]
) -> torch.Tensor:
    """Runs a batch of RGB frames through the upscaling model"""
    batch = torch.from_numpy(np.stack(frames_rgb)).cuda()  # (N, H, W, 3) uint8
    batch = batch.permute(0, 3, 1, 2).half().div_(255.0)  # (N, 3, H, W) fp16

    with torch.no_grad():
        with torch.autocast(device_type="cuda"):
            return model(batch)  # (N, 3, out_H, out_W)


_LUMA_WEIGHT_R, _LUMA_WEIGHT_B = 0.299, 0.114
_LUMA_WEIGHT_G = 1.0 - _LUMA_WEIGHT_R - _LUMA_WEIGHT_B

# limited ("TV") range
_Y_MIN, _Y_MAX = 16, 235
_C_MIN, _C_MAX = 16, 240


def _rgb_to_yuv420(rgb: torch.Tensor) -> list[np.ndarray]:
    """Converts a batch of RGB tensors in [0, 1] to BT.601 limited-range YUV420p,
    averaging U and V over each 2x2 block of pixels.
    """
    r, g, b = rgb.clamp(0, 1).unbind(dim=1)  # each (N, H, W)

    luma = _LUMA_WEIGHT_R * r + _LUMA_WEIGHT_G * g + _LUMA_WEIGHT_B * b
    # colour diffs scaled into -0.5..0.5 so they can be stretched to U/V range
    blue_diff = (b - luma) / (2 * (1 - _LUMA_WEIGHT_B))
    red_diff = (r - luma) / (2 * (1 - _LUMA_WEIGHT_R))

    # limited range: Y uses 16..235, U/V use 16..240 centered on 128
    y = _Y_MIN + (_Y_MAX - _Y_MIN) * luma
    u, v = F.avg_pool2d(torch.stack([blue_diff, red_diff], dim=1), 2).unbind(dim=1)
    u, v = (
        ((_C_MIN + _C_MAX) // 2) + (_C_MAX - _C_MIN) * u,
        ((_C_MIN + _C_MAX) // 2) + (_C_MAX - _C_MIN) * v,
    )

    planar = torch.cat([y.flatten(1), u.flatten(1), v.flatten(1)], dim=1)
    return list(planar.byte().cpu().numpy())
