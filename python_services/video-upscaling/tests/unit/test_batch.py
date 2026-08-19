from contextlib import contextmanager
from queue import Queue
from typing import Generator, Optional
from unittest.mock import MagicMock, patch
import numpy as np
import pytest
import torch


def _make_bgr_frame(r: float, g: float, b: float, h: int = 4, w: int = 4) -> np.ndarray:
    """Return a solid-colour BGR frame as uint8 in [0, 255]."""
    frame = np.zeros((h, w, 3), dtype=np.uint8)
    frame[:, :, 0] = int(b * 255)  # B channel
    frame[:, :, 1] = int(g * 255)  # G channel
    frame[:, :, 2] = int(r * 255)  # R channel
    return frame


def _model_passthrough(batch: torch.Tensor) -> torch.Tensor:
    """Fake model that returns the input unchanged (already in [0,1] RGB fp16)."""
    return batch.float()  # autocast may produce fp16; cast to float for math


@contextmanager
def _patch_cuda() -> Generator[None, None, None]:
    """
    Patch out the two CUDA-specific calls in _infer_batch so tests run on CPU:
      - torch.Tensor.cuda  →  returns self
      - torch.autocast     →  no-op context manager
    """

    @contextmanager
    def _fake_autocast(**_kwargs: object) -> Generator[None, None, None]:
        yield

    with (
        patch.object(torch.Tensor, "cuda", lambda self: self),
        patch("torch.autocast", _fake_autocast),
    ):
        yield


class TestInferBatch:
    def _run(self, frames: list[np.ndarray]) -> list[bytes]:
        from src.processing.batch import _infer_batch

        with _patch_cuda():
            return _infer_batch(_model_passthrough, frames)  # type: ignore[arg-type]

    def test_returns_one_bytes_object_per_frame(self) -> None:
        frames = [_make_bgr_frame(1, 0, 0)] * 3
        results = self._run(frames)
        assert len(results) == 3
        assert all(isinstance(r, bytes) for r in results)

    def test_output_length_matches_yuv420p(self) -> None:
        h, w = 4, 4
        frame = _make_bgr_frame(1, 0, 0, h, w)
        results = self._run([frame])
        # YUV420p: Y plane (h*w) + U plane (h/2 * w/2) + V plane (h/2 * w/2)
        expected = h * w + (h // 2) * (w // 2) + (h // 2) * (w // 2)
        assert len(results[0]) == expected

    @pytest.mark.parametrize(
        "name,rgb,expected_y,expected_u,expected_v",
        [
            # ground-truth values computed from the BT.601 coefficients in batch.py
            ("black", (0.0, 0.0, 0.0), 16, 128, 128),
            ("white", (1.0, 1.0, 1.0), 235, 128, 128),
            ("red", (1.0, 0.0, 0.0), 81, 90, 240),
            ("green", (0.0, 1.0, 0.0), 144, 53, 34),
        ],
    )
    def test_yuv_values_match_bt601(
        self,
        name: str,
        rgb: tuple[float, float, float],
        expected_y: int,
        expected_u: int,
        expected_v: int,
    ) -> None:
        h, w = 4, 4
        r, g, b = rgb
        frame = _make_bgr_frame(r, g, b, h, w)
        result = self._run([frame])[0]

        raw = np.frombuffer(result, dtype=np.uint8)
        y_plane = raw[: h * w].reshape(h, w)
        u_plane = raw[h * w : h * w + (h // 2) * (w // 2)].reshape(h // 2, w // 2)
        v_plane = raw[h * w + (h // 2) * (w // 2) :].reshape(h // 2, w // 2)

        # Allow ±2 for fp16 rounding in the model passthrough
        assert abs(int(y_plane[0, 0]) - expected_y) <= 2, f"{name} Y mismatch"
        assert abs(int(u_plane[0, 0]) - expected_u) <= 2, f"{name} U mismatch"
        assert abs(int(v_plane[0, 0]) - expected_v) <= 2, f"{name} V mismatch"

    def test_y_plane_is_clamped_to_16_235(self) -> None:
        # Black should hit the Y floor of 16 (not 0)
        frame = _make_bgr_frame(0.0, 0.0, 0.0)
        result = self._run([frame])[0]
        h, w = frame.shape[:2]
        y_plane = np.frombuffer(result[: h * w], dtype=np.uint8)
        assert np.all(y_plane >= 16)
        assert np.all(y_plane <= 235)

    def test_uv_planes_are_clamped_to_16_240(self) -> None:
        h, w = 4, 4
        frame = _make_bgr_frame(
            1.0, 0.0, 0.0, h, w
        )  # red → U near floor, V near ceiling
        result = self._run([frame])[0]
        uv = np.frombuffer(result[h * w :], dtype=np.uint8)
        assert np.all(uv >= 16)
        assert np.all(uv <= 240)

    def test_multiple_frames_are_independent(self) -> None:
        red_frame = _make_bgr_frame(1.0, 0.0, 0.0)
        green_frame = _make_bgr_frame(0.0, 1.0, 0.0)
        results = self._run([red_frame, green_frame])

        h, w = red_frame.shape[:2]
        y_red = np.frombuffer(results[0][: h * w], dtype=np.uint8)
        y_green = np.frombuffer(results[1][: h * w], dtype=np.uint8)

        assert y_red[0] != y_green[0]


class TestFlushBatch:
    def _run_flush(
        self,
        frames: list[np.ndarray],
        infer_results: list[bytes],
    ) -> tuple[float, float, int]:
        from src.processing.batch import flush_batch

        mock_upsampler = MagicMock()
        encode_queue: Queue[Optional[bytes]] = Queue()

        with patch("src.processing.batch._infer_batch", return_value=infer_results):
            timing = flush_batch(mock_upsampler, frames, encode_queue)

        self._queue = encode_queue
        return timing

    def test_returns_three_element_tuple(self) -> None:
        result = self._run_flush([_make_bgr_frame(1, 0, 0)], [b"frame"])
        assert len(result) == 3

    def test_frame_count_matches_input(self) -> None:
        frames = [_make_bgr_frame(1, 0, 0)] * 5
        _, _, count = self._run_flush(frames, [b"x"] * 5)
        assert count == 5

    def test_all_results_enqueued(self) -> None:
        fake_results = [b"a", b"b", b"c"]
        self._run_flush([MagicMock()] * 3, fake_results)
        queued = [self._queue.get_nowait() for _ in range(3)]
        assert queued == fake_results

    def test_queue_is_empty_after_flush(self) -> None:
        self._run_flush([MagicMock()], [b"frame"])
        self._queue.get_nowait()  # drain the one frame
        assert self._queue.empty()

    def test_inference_time_is_non_negative(self) -> None:
        infer_time, _, _ = self._run_flush([MagicMock()], [b"x"])
        assert infer_time >= 0

    def test_enqueue_time_is_non_negative(self) -> None:
        _, enqueue_time, _ = self._run_flush([MagicMock()], [b"x"])
        assert enqueue_time >= 0

    def test_infer_batch_called_with_model_and_frames(self) -> None:
        from src.processing.batch import flush_batch

        mock_upsampler = MagicMock()
        frames = [_make_bgr_frame(1, 0, 0)]
        encode_queue: Queue[Optional[bytes]] = Queue()

        with patch(
            "src.processing.batch._infer_batch", return_value=[b"x"]
        ) as mock_infer:
            flush_batch(mock_upsampler, frames, encode_queue)

        mock_infer.assert_called_once_with(mock_upsampler.model, frames)
