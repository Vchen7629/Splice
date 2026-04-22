from typing import Any
from pathlib import Path
from unittest.mock import patch
from unittest.mock import MagicMock
from subprocess import CalledProcessError
from src.processing.video import video_decoder
from src.processing.video import video_encoder
from src.processing.video import video_upscale
from src.processing.video import video_downscale
from src.processing.video import extract_video_info
from src.processing.video import recombine_video_audio
from tests.fixtures.processing_helpers import make_fake_decoder
import pytest
import subprocess
import numpy as np


@pytest.mark.parametrize("bad_path", ["", None])
def test_extract_video_info_raises_type_error_for_missing_path(
    bad_path: str | None,
) -> None:
    with pytest.raises(TypeError, match="Missing video_path input"):
        extract_video_info(bad_path)  # type: ignore[arg-type]


@pytest.mark.parametrize("fps", [0, -1, -30.0])
def test_video_encoder_raises_for_invalid_fps(fps: float) -> None:
    with pytest.raises(ValueError, match="fps cant be negative or 0"):
        video_encoder(fps, 1280, 720, "/tmp/out.mp4")


@pytest.mark.parametrize("out_w", [0, -1])
def test_video_encoder_raises_for_invalid_width(out_w: int) -> None:
    with pytest.raises(ValueError, match="out_w cant be negative or 0"):
        video_encoder(24.0, out_w, 720, "/tmp/out.mp4")


@pytest.mark.parametrize("out_h", [0, -1, None])
def test_video_encoder_raises_for_invalid_height(out_h: int | None) -> None:
    with pytest.raises(ValueError, match="out_h cant be negative or 0"):
        video_encoder(24.0, 1280, out_h, "/tmp/out.mp4")  # type: ignore[arg-type]


def test_video_downscale_raises_runtime_error_when_ffmpeg_fails() -> None:
    with patch(
        "src.processing.video.subprocess.run",
        side_effect=CalledProcessError(1, "ffmpeg", stderr=b"error"),
    ):
        with pytest.raises(RuntimeError, match="ffmpeg downscale failed"):
            video_downscale("/tmp/input.mp4", "480p", "/tmp/out.mp4")


def test_video_decoder_calls_popen_with_video_path() -> None:
    with patch(
        "src.processing.video.subprocess.Popen", return_value=MagicMock()
    ) as mock_popen:
        video_decoder("/tmp/input.mp4")

        args = mock_popen.call_args[0][0]
        assert "/tmp/input.mp4" in args


def test_video_decoder_returns_popen_instance() -> None:
    mock_proc = MagicMock()
    with patch("src.processing.video.subprocess.Popen", return_value=mock_proc):
        assert video_decoder("/tmp/input.mp4") is mock_proc


def test_video_decoder_opens_stdout_pipe() -> None:
    with patch(
        "src.processing.video.subprocess.Popen", return_value=MagicMock()
    ) as mock_popen:
        video_decoder("/tmp/input.mp4")

        assert mock_popen.call_args[1]["stdout"] == subprocess.PIPE


def test_video_decoder_outputs_rgb24() -> None:
    with patch(
        "src.processing.video.subprocess.Popen", return_value=MagicMock()
    ) as mock_popen:
        video_decoder("/tmp/input.mp4")

        args = mock_popen.call_args[0][0]
        assert "rgb24" in args


def test_recombine_video_audio_calls_subprocess_run() -> None:
    with patch("src.processing.video.subprocess.run") as mock_run:
        recombine_video_audio("/tmp/original.mp4", "/tmp/final.mp4")

        mock_run.assert_called_once()


def test_recombine_video_audio_passes_correct_paths() -> None:
    with patch("src.processing.video.subprocess.run") as mock_run:
        recombine_video_audio("/tmp/original.mp4", "/tmp/final.mp4")

        args = mock_run.call_args[0][0]
        assert "/tmp/upscaled_noaudio.mp4" in args
        assert "/tmp/original.mp4" in args
        assert "/tmp/final.mp4" in args


@pytest.mark.parametrize(
    "n_frames,batch_size",
    [
        (4, 4),  # exactly one full batch
        (5, 4),  # one full batch + partial remainder
        (3, 4),  # only a partial batch
    ],
)
def test_video_upscale_flushes_all_frames(
    video_upscale_patches: dict[str, Any], n_frames: int, batch_size: int
) -> None:
    w, h = 64, 64
    frames = [np.zeros((h, w, 3), dtype=np.uint8) for _ in range(n_frames)]
    video_upscale_patches["decoder"].return_value = make_fake_decoder(frames)
    video_upscale_patches["info"].return_value = (w, h, 24.0, 22)
    video_upscale_patches["settings"].BATCH_SIZE = batch_size

    flushed: list[int] = []

    def capture_flush(
        upsampler: object, pending: list[np.ndarray], queue: object
    ) -> tuple[float, float, int]:
        flushed.append(len(pending))
        return 0.0, 0.0, len(pending)

    video_upscale_patches["flush"].side_effect = capture_flush

    video_upscale("/tmp/input.mp4", "/tmp/output.mp4", Path("/weights/model.pth"), 2)

    assert sum(flushed) == n_frames


def test_video_upscale_loads_model_with_correct_args(
    video_upscale_patches: dict[str, Any],
) -> None:
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    model_path = Path("/weights/model.pth")
    video_upscale("/tmp/input.mp4", "/tmp/output.mp4", model_path, 2)

    video_upscale_patches["load"].assert_called_once_with(model_path, 2)


def test_video_upscale_encoder_gets_scaled_dimensions(
    video_upscale_patches: dict[str, Any],
) -> None:
    w, h, scale = 64, 64, 2
    video_upscale_patches["info"].return_value = (w, h, 24.0, 22)
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    video_upscale(
        "/tmp/input.mp4", "/tmp/output.mp4", Path("/weights/model.pth"), scale
    )

    video_upscale_patches["encoder"].assert_called_once_with(
        24.0, w * scale, h * scale, "/tmp/upscaled_noaudio.mp4"
    )


def test_video_upscale_calls_recombine_with_output_path(
    video_upscale_patches: dict[str, Any],
) -> None:
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    video_upscale("/tmp/input.mp4", "/tmp/output.mp4", Path("/weights/model.pth"), 2)

    video_upscale_patches["recombine"].assert_called_once_with(
        "/tmp/input.mp4", "/tmp/output.mp4"
    )
