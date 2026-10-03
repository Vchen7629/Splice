from queue import Queue
from typing import Optional
from unittest.mock import MagicMock, patch

import numpy as np
import pytest
import torch

from src.processing.batch import _rgb_to_yuv420, _upscale_frames, flush_batch


def _make_rgb_frame(r: float, g: float, b: float, h: int = 4, w: int = 4) -> np.ndarray:
    """Return a solid-colour RGB frame as uint8 in [0, 255]."""
    frame = np.zeros((h, w, 3), dtype=np.uint8)
    frame[:, :, 0] = int(r * 255)  # R channel
    frame[:, :, 1] = int(g * 255)  # G channel
    frame[:, :, 2] = int(b * 255)  # B channel
    return frame


def _make_rgb_tensor(
    *colors: tuple[float, float, float], h: int = 4, w: int = 4
) -> torch.Tensor:
    """Return an (N, 3, H, W) float tensor with one solid colour per frame."""
    return torch.stack([torch.tensor(c).view(3, 1, 1).expand(3, h, w) for c in colors])


def _split_planes(
    data: bytes, h: int, w: int
) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    """Split one frame of planar YUV420p bytes into its Y, U and V planes."""
    raw = np.frombuffer(data, dtype=np.uint8)
    chroma = (h // 2) * (w // 2)
    y = raw[: h * w].reshape(h, w)
    u = raw[h * w : h * w + chroma].reshape(h // 2, w // 2)
    v = raw[h * w + chroma :].reshape(h // 2, w // 2)
    return y, u, v


def _upscale_with_passthrough_model(frames: list[np.ndarray]) -> torch.Tensor:
    """Run _upscale_frames on CPU with a model that returns its input, and return that input."""
    with patch("torch.Tensor.cuda", lambda self: self), patch("torch.autocast"):
        return _upscale_frames(lambda batch: batch, frames)  # type: ignore[arg-type]


def _run_flush(
    frames: list[np.ndarray], yuv_results: list[bytes]
) -> tuple[tuple[float, float, int], Queue[Optional[bytes]]]:
    """Run flush_batch with the model and yuv conversion mocked, returning (timing, queue)."""
    encode_queue: Queue[Optional[bytes]] = Queue()

    with (
        patch("src.processing.batch._upscale_frames"),
        patch("src.processing.batch._rgb_to_yuv420", return_value=yuv_results),
    ):
        timing = flush_batch(MagicMock(), frames, encode_queue)

    return timing, encode_queue


def test_upscale_frames_model_input_is_nchw_fp16() -> None:
    batch = _upscale_with_passthrough_model([_make_rgb_frame(1, 0, 0, h=4, w=6)] * 3)
    assert batch.shape == (3, 3, 4, 6)
    assert batch.dtype == torch.float16


def test_upscale_frames_keeps_rgb_channel_order() -> None:
    # pure red scaled to 1.0 must land in channel 0, with no BGR swap
    batch = _upscale_with_passthrough_model([_make_rgb_frame(1, 0, 0)])
    assert batch[0, 0].min() == 1.0
    assert batch[0, 1].max() == 0.0
    assert batch[0, 2].max() == 0.0


def test_rgb_to_yuv420_output_matches_yuv420p_layout() -> None:
    h, w = 4, 6
    results = _rgb_to_yuv420(_make_rgb_tensor((1, 0, 0), (0, 1, 0), h=h, w=w))

    assert len(results) == 2
    # Y plane (h*w) + U plane (h/2 * w/2) + V plane (h/2 * w/2)
    assert all(len(r) == h * w + 2 * (h // 2) * (w // 2) for r in results)


@pytest.mark.parametrize(
    "name,rgb,expected_y,expected_u,expected_v",
    [
        # ground-truth values computed from the BT.601 limited-range equations
        ("black", (0.0, 0.0, 0.0), 16, 128, 128),
        ("white", (1.0, 1.0, 1.0), 235, 128, 128),
        ("red", (1.0, 0.0, 0.0), 81, 90, 240),
        ("green", (0.0, 1.0, 0.0), 144, 53, 34),
        ("blue", (0.0, 0.0, 1.0), 40, 240, 109),
    ],
)
def test_rgb_to_yuv420_values_match_bt601(
    name: str,
    rgb: tuple[float, float, float],
    expected_y: int,
    expected_u: int,
    expected_v: int,
) -> None:
    h, w = 4, 4
    y, u, v = _split_planes(_rgb_to_yuv420(_make_rgb_tensor(rgb, h=h, w=w))[0], h, w)

    # ±1 allows for float rounding before the uint8 cast
    assert abs(int(y[0, 0]) - expected_y) <= 1, f"{name} Y mismatch"
    assert abs(int(u[0, 0]) - expected_u) <= 1, f"{name} U mismatch"
    assert abs(int(v[0, 0]) - expected_v) <= 1, f"{name} V mismatch"


@pytest.mark.parametrize("value", [-1.0, 2.0])
def test_rgb_to_yuv420_clamps_out_of_range_input(value: float) -> None:
    # model output outside [0, 1] must not push planes outside the limited range
    h, w = 4, 4
    (data,) = _rgb_to_yuv420(_make_rgb_tensor((value, 0.5, value), h=h, w=w))
    y, u, v = _split_planes(data, h, w)

    assert 16 <= y.min() and y.max() <= 235
    assert 16 <= u.min() and u.max() <= 240
    assert 16 <= v.min() and v.max() <= 240


def test_flush_batch_enqueues_all_results_and_returns_frame_count() -> None:
    fake_results = [b"a", b"b", b"c"]
    (_, _, count), queue = _run_flush([_make_rgb_frame(1, 0, 0)] * 3, fake_results)

    assert count == 3
    assert [queue.get_nowait() for _ in range(3)] == fake_results
    assert queue.empty()


def test_flush_batch_passes_upscaled_frames_to_yuv_conversion() -> None:
    mock_upsampler = MagicMock()
    frames = [_make_rgb_frame(1, 0, 0)]

    with (
        patch("src.processing.batch._upscale_frames") as mock_upscale,
        patch("src.processing.batch._rgb_to_yuv420", return_value=[b"x"]) as mock_yuv,
    ):
        flush_batch(mock_upsampler, frames, Queue())

    mock_upscale.assert_called_once_with(mock_upsampler.model, frames)
    mock_yuv.assert_called_once_with(mock_upscale.return_value)
