from threading import Event
from typing import Any
from pathlib import Path
from unittest.mock import MagicMock, patch
from subprocess import CalledProcessError
from src.processing.video import (
    video_decoder,
    video_encoder,
    video_upscale,
    video_downscale,
    extract_video_info,
    recombine_video_audio,
)
from tests.fixtures.processing_helpers import make_fake_decoder
import pytest
import subprocess
import numpy as np


MOCK_CANCEL_EVENT = MagicMock(spec=Event)
MOCK_CANCEL_EVENT.is_set.return_value = False


def _fake_recombine_proc() -> MagicMock:
    """A Popen stand-in with no progress lines and a clean exit"""
    proc = MagicMock()
    proc.stdout = iter([])
    proc.wait.return_value = 0
    return proc


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
    with (
        patch("src.processing.video.torch.cuda.is_available", return_value=False),
        patch(
            "src.processing.video.subprocess.Popen", return_value=MagicMock()
        ) as mock_popen,
    ):
        video_decoder("/tmp/input.mp4")

        args = mock_popen.call_args[0][0]
        assert "/tmp/input.mp4" in args


def test_video_decoder_returns_popen_instance() -> None:
    mock_proc = MagicMock()
    with (
        patch("src.processing.video.torch.cuda.is_available", return_value=False),
        patch("src.processing.video.subprocess.Popen", return_value=mock_proc),
    ):
        assert video_decoder("/tmp/input.mp4") is mock_proc


def test_video_decoder_opens_stdout_pipe() -> None:
    with (
        patch("src.processing.video.torch.cuda.is_available", return_value=False),
        patch(
            "src.processing.video.subprocess.Popen", return_value=MagicMock()
        ) as mock_popen,
    ):
        video_decoder("/tmp/input.mp4")

        assert mock_popen.call_args[1]["stdout"] == subprocess.PIPE


def test_video_decoder_outputs_rgb24() -> None:
    with (
        patch("src.processing.video.torch.cuda.is_available", return_value=False),
        patch(
            "src.processing.video.subprocess.Popen", return_value=MagicMock()
        ) as mock_popen,
    ):
        video_decoder("/tmp/input.mp4")

        args = mock_popen.call_args[0][0]
        assert "rgb24" in args


def test_video_upscale_encoder_uses_job_scoped_temp_path(
    video_upscale_patches: dict[str, Any],
) -> None:
    """2 jobs should not write their output to same path which would cause leakage between diff jobs"""
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])
    video_upscale(
        MOCK_CANCEL_EVENT, "job_id1", "/tmp/input.mp4", Path("/weights/model.pth"), 2
    )
    out_path_a = video_upscale_patches["encoder"].call_args[0][3]

    video_upscale_patches["decoder"].return_value = make_fake_decoder([])
    video_upscale(
        MOCK_CANCEL_EVENT, "job_id2", "/tmp/input.mp4", Path("/weights/model.pth"), 2
    )
    out_path_b = video_upscale_patches["encoder"].call_args[0][3]

    assert out_path_a == "/tmp/upscaled_noaudio-job_id1.mp4"
    assert out_path_b == "/tmp/upscaled_noaudio-job_id2.mp4"


def test_recombine_video_audio_reads_job_scoped_temp_path() -> None:
    with (
        patch("src.processing.video.subprocess.run") as mock_run,
        patch(
            "src.processing.video.subprocess.Popen",
            side_effect=lambda *a, **kw: _fake_recombine_proc(),
        ) as mock_popen,
    ):
        mock_run.return_value.stdout = "10.0"

        recombine_video_audio("job_id1", "/tmp/original.mp4", "/tmp/final.mp4")
        input_a = mock_popen.call_args[0][0][mock_popen.call_args[0][0].index("-i") + 1]

        recombine_video_audio("job_id2", "/tmp/original.mp4", "/tmp/final.mp4")
        input_b = mock_popen.call_args[0][0][mock_popen.call_args[0][0].index("-i") + 1]

    assert input_a == "/tmp/upscaled_noaudio-job_id1.mp4"
    assert input_b == "/tmp/upscaled_noaudio-job_id2.mp4"


def test_recombine_video_audio_calls_subprocess_popen() -> None:
    with (
        patch("src.processing.video.subprocess.run") as mock_run,
        patch(
            "src.processing.video.subprocess.Popen", return_value=_fake_recombine_proc()
        ) as mock_popen,
    ):
        mock_run.return_value.stdout = "10.0"

        recombine_video_audio("job_id1", "/tmp/original.mp4", "/tmp/final.mp4")

        mock_popen.assert_called_once()


def test_recombine_video_audio_passes_correct_paths() -> None:
    with (
        patch("src.processing.video.subprocess.run") as mock_run,
        patch(
            "src.processing.video.subprocess.Popen", return_value=_fake_recombine_proc()
        ) as mock_popen,
    ):
        mock_run.return_value.stdout = "10.0"

        recombine_video_audio("job_id1", "/tmp/original.mp4", "/tmp/final.mp4")

        args = mock_popen.call_args[0][0]
        assert "/tmp/upscaled_noaudio-job_id1.mp4" in args
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

    video_upscale(
        MOCK_CANCEL_EVENT, "jobid1", "/tmp/input.mp4", Path("/weights/model.pth"), 2
    )

    assert sum(flushed) == n_frames


def test_video_upscale_loads_model_with_correct_args(
    video_upscale_patches: dict[str, Any],
) -> None:
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    model_path = Path("/weights/model.pth")
    video_upscale(MOCK_CANCEL_EVENT, "job_id1", "/tmp/input.mp4", model_path, 2)

    video_upscale_patches["load"].assert_called_once_with(model_path, 2)


def test_video_upscale_encoder_gets_scaled_dimensions(
    video_upscale_patches: dict[str, Any],
) -> None:
    w, h, scale = 64, 64, 2
    video_upscale_patches["info"].return_value = (w, h, 24.0, 22)
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    video_upscale(
        MOCK_CANCEL_EVENT,
        "job_id1",
        "/tmp/input.mp4",
        Path("/weights/model.pth"),
        scale,
    )

    video_upscale_patches["encoder"].assert_called_once_with(
        24.0, w * scale, h * scale, "/tmp/upscaled_noaudio-job_id1.mp4"
    )
