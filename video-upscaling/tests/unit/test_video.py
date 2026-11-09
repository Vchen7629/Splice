import subprocess
import time
from pathlib import Path
from subprocess import CalledProcessError
from threading import Event
from typing import Any, Generator
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from core import JobCancelledError, output_dir
from processing.video import recombine_video_audio, video_downscale, video_upscale
from tests.fixtures.processing import running_proc

MODEL = Path("/weights/model.pth")


def make_fake_decoder(frames: list[np.ndarray]) -> MagicMock:
    """Build a mock decoder whose stdout yields the given raw rgb frames then EOF."""
    stdout = MagicMock()
    stdout.read.side_effect = [f.tobytes() for f in frames] + [b""]
    decoder = MagicMock()
    decoder.stdout = stdout
    return decoder


def _fake_recombine_proc() -> MagicMock:
    """A Popen stand-in with no progress lines and a clean exit"""
    proc = MagicMock()
    proc.stdout = MagicMock()
    proc.stdout.__iter__.return_value = iter([])
    proc.wait.return_value = 0
    return proc


@pytest.fixture
def mock_pipeline(monkeypatch: pytest.MonkeyPatch) -> tuple[MagicMock, MagicMock]:
    mock_decoder = running_proc()
    mock_decoder.stdout = MagicMock()
    mock_encoder = running_proc()

    monkeypatch.setattr(
        "processing.video._extract_video_info", lambda p: (100, 100, 30.0, 10)
    )
    monkeypatch.setattr("processing.video._load_model", lambda *a: MagicMock())
    monkeypatch.setattr("processing.video._video_decoder", lambda p: mock_decoder)
    monkeypatch.setattr("processing.video._video_encoder", lambda *a: mock_encoder)
    monkeypatch.setattr(
        "processing.video.threading.Thread", lambda *a, **kw: MagicMock()
    )
    return mock_decoder, mock_encoder


@pytest.fixture
def video_upscale_patches() -> Generator[dict[str, Any], Any, None]:
    """Patches all external dependencies used by video_upscale."""
    with (
        patch(
            "processing.video._extract_video_info", return_value=(64, 64, 24.0, 0)
        ) as mock_info,
        patch("processing.video._load_model", return_value=MagicMock()) as mock_load,
        patch("processing.video._video_decoder") as mock_decoder,
        patch("processing.video._video_encoder", return_value=MagicMock()) as mock_enc,
        patch(
            "processing.video.flush_and_upscale_batch", return_value=(0.0, 0.0, 0)
        ) as mock_flush,
        patch("processing.video.encoder_worker"),
        patch("processing.video.threading.Thread", return_value=MagicMock()),
        patch("processing.video.settings") as mock_settings,
    ):
        mock_settings.BATCH_SIZE = 4
        yield {
            "info": mock_info,
            "load": mock_load,
            "decoder": mock_decoder,
            "encoder": mock_enc,
            "flush": mock_flush,
            "settings": mock_settings,
        }


@pytest.fixture
def recombine_popen() -> Generator[MagicMock, Any, None]:
    with (
        patch("processing.ffmpeg.run") as mock_run,
        patch(
            "processing.video.Popen", return_value=_fake_recombine_proc()
        ) as mock_popen,
    ):
        mock_run.return_value.stdout = "10.0"
        yield mock_popen


def test_video_downscale_raises_runtime_error_when_ffmpeg_fails() -> None:
    with patch(
        "processing.ffmpeg.run",
        side_effect=CalledProcessError(1, "ffmpeg", stderr=b"error"),
    ):
        with pytest.raises(RuntimeError, match="ffmpeg downscale failed"):
            video_downscale(Event(), "/tmp/input.mp4", "480p", "/tmp/out.mp4")


def test_video_downscale_raises_before_starting_ffmpeg_when_already_cancelled(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    cancel_event = Event()
    cancel_event.set()

    mock_popen = MagicMock()
    monkeypatch.setattr("processing.video.Popen", mock_popen)

    with pytest.raises(JobCancelledError):
        video_downscale(cancel_event, "/tmp/input.mp4", "480p", "/tmp/out.mp4")

    mock_popen.assert_not_called()


def test_video_downscale_kills_ffmpeg_and_raises_when_cancelled_mid_run(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    cancel_event = MagicMock(spec=Event)
    cancel_event.is_set.side_effect = [False, True]

    mock_proc = MagicMock()
    mock_proc.stdout.__iter__.return_value = iter(["out_time=00:00:01.00\n"])
    mock_proc.poll.return_value = None

    monkeypatch.setattr("processing.video._probe_duration_s", lambda p: 10.0)
    monkeypatch.setattr("processing.video.Popen", lambda *a, **kw: mock_proc)

    with pytest.raises(JobCancelledError):
        video_downscale(cancel_event, "/tmp/input.mp4", "480p", "/tmp/out.mp4")

    mock_proc.kill.assert_called()
    mock_proc.wait.assert_called()
    mock_proc.stdout.close.assert_called_once()


def test_video_upscale_encoder_uses_job_scoped_temp_path(
    video_upscale_patches: dict[str, Any],
) -> None:
    """2 jobs should not write their output to same path which would cause leakage between diff jobs"""
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])
    video_upscale(Event(), "job_id1", "/tmp/input.mp4", MODEL, 2, "1080p")
    out_path_a = video_upscale_patches["encoder"].call_args[0][3]

    video_upscale_patches["decoder"].return_value = make_fake_decoder([])
    video_upscale(Event(), "job_id2", "/tmp/input.mp4", MODEL, 2, "1080p")
    out_path_b = video_upscale_patches["encoder"].call_args[0][3]

    assert out_path_a == f"{output_dir('job_id1')}/upscaled_noaudio.mp4"
    assert out_path_b == f"{output_dir('job_id2')}/upscaled_noaudio.mp4"


def test_video_upscale_kills_processes_and_raises_when_timeout_exceeded(
    mock_pipeline: tuple[MagicMock, MagicMock], monkeypatch: pytest.MonkeyPatch
) -> None:
    mock_decoder, mock_encoder = mock_pipeline
    monkeypatch.setattr("processing.video.settings.FFMPEG_TIMEOUT_GRACE_S", 0)
    monkeypatch.setattr("processing.video.settings.UPSCALE_2X_TIMEOUT_S_PER_FRAME", 0)
    # stand in for a read blocked on a pipe so the watcher's first 0.1s poll fires mid-read
    mock_decoder.stdout.read.side_effect = lambda n: time.sleep(0.3) or b""

    with pytest.raises(subprocess.TimeoutExpired):
        video_upscale(Event(), "job-1", "video.mp4", MODEL, 2, "1080p")

    mock_decoder.kill.assert_called_once()
    mock_encoder.kill.assert_called()


def test_recombine_video_audio_reads_job_scoped_temp_path(
    recombine_popen: MagicMock,
) -> None:
    recombine_video_audio("job_id1", "/tmp/original.mp4", "/tmp/final.mp4")
    args_a = recombine_popen.call_args[0][0]

    recombine_video_audio("job_id2", "/tmp/original.mp4", "/tmp/final.mp4")
    args_b = recombine_popen.call_args[0][0]

    assert (
        args_a[args_a.index("-i") + 1]
        == f"{output_dir('job_id1')}/upscaled_noaudio.mp4"
    )
    assert (
        args_b[args_b.index("-i") + 1]
        == f"{output_dir('job_id2')}/upscaled_noaudio.mp4"
    )


def test_recombine_video_audio_passes_correct_paths(
    recombine_popen: MagicMock,
) -> None:
    recombine_video_audio("job_id1", "/tmp/original.mp4", "/tmp/final.mp4")

    recombine_popen.assert_called_once()
    args = recombine_popen.call_args[0][0]
    assert f"{output_dir('job_id1')}/upscaled_noaudio.mp4" in args
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

    video_upscale(Event(), "jobid1", "/tmp/input.mp4", MODEL, 2, "1080p")

    assert sum(flushed) == n_frames


def test_video_upscale_loads_model_with_correct_args(
    video_upscale_patches: dict[str, Any],
) -> None:
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    video_upscale(Event(), "job_id1", "/tmp/input.mp4", MODEL, 2, "1080p")

    video_upscale_patches["load"].assert_called_once_with(MODEL, 2)


def test_video_upscale_encoder_gets_scaled_dimensions(
    video_upscale_patches: dict[str, Any],
) -> None:
    w, h, scale = 64, 64, 2
    video_upscale_patches["info"].return_value = (w, h, 24.0, 22)
    video_upscale_patches["decoder"].return_value = make_fake_decoder([])

    video_upscale(Event(), "job_id1", "/tmp/input.mp4", MODEL, scale, "1080p")

    video_upscale_patches["encoder"].assert_called_once_with(
        24.0,
        w * scale,
        h * scale,
        f"{output_dir('job_id1')}/upscaled_noaudio.mp4",
        "1080p",
    )


def test_video_upscale_kill_processes_and_raises_when_cancelled(
    mock_pipeline: tuple[MagicMock, MagicMock],
) -> None:
    mock_decoder, mock_encoder = mock_pipeline
    cancel_event = Event()
    cancel_event.set()

    with pytest.raises(JobCancelledError):
        video_upscale(cancel_event, "job-1", "video.mp4", MODEL, 2, "1080p")

    mock_decoder.stdout.close.assert_called_once()
    mock_decoder.kill.assert_called_once()
    mock_encoder.kill.assert_called_once()


def test_video_upscale_kill_processes_and_raises_when_encoder_fails(
    mock_pipeline: tuple[MagicMock, MagicMock], monkeypatch: pytest.MonkeyPatch
) -> None:
    mock_decoder, mock_encoder = mock_pipeline

    fail_event = MagicMock(spec=Event)
    fail_event.is_set.return_value = True
    monkeypatch.setattr("processing.video.Event", lambda: fail_event)

    with pytest.raises(Exception, match="encoder failed"):
        video_upscale(Event(), "job-1", "video.mp4", MODEL, 2, "1080p")

    mock_decoder.stdout.close.assert_called_once()
    mock_decoder.kill.assert_called_once()
    mock_encoder.kill.assert_called_once()
