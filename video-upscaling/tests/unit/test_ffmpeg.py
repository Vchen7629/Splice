import subprocess
from threading import Event
from typing import Any, Generator
from unittest.mock import MagicMock, patch

import pytest

from processing.ffmpeg import (
    _extract_video_info,
    _probe_duration_s,
    _run_ffprobe,
    _video_decoder,
    _video_encoder,
    kill_on_deadline,
)
from tests.fixtures.processing import running_proc


@pytest.fixture
def decoder_popen() -> Generator[MagicMock, Any, None]:
    with (
        patch("processing.ffmpeg.is_available", return_value=False),
        patch("processing.ffmpeg.Popen", return_value=MagicMock()) as m,
    ):
        yield m


@pytest.mark.parametrize("bad_path", ["", None])
def test_run_ffprobe_raises_type_error_for_missing_path(
    bad_path: str | None,
) -> None:
    with pytest.raises(TypeError, match="Missing video_path input"):
        _run_ffprobe(bad_path)  # type: ignore[arg-type]


@pytest.mark.parametrize("fps", [0, -1, -30.0])
def test__video_encoder_raises_for_invalid_fps(fps: float) -> None:
    with pytest.raises(ValueError, match="fps cant be negative or 0"):
        _video_encoder(fps, 1280, 720, "/tmp/out.mp4", "720p")


@pytest.mark.parametrize("out_w", [0, -1])
def test__video_encoder_raises_for_invalid_width(out_w: int) -> None:
    with pytest.raises(ValueError, match="out_w cant be negative or 0"):
        _video_encoder(24.0, out_w, 720, "/tmp/out.mp4", "720p")


@pytest.mark.parametrize("out_h", [0, -1, None])
def test__video_encoder_raises_for_invalid_height(out_h: int | None) -> None:
    with pytest.raises(ValueError, match="out_h cant be negative or 0"):
        _video_encoder(24.0, 1280, out_h, "/tmp/out.mp4", "720p")  # type: ignore[arg-type]


def test__video_decoder_calls_popen_with_video_path(decoder_popen: MagicMock) -> None:
    _video_decoder("/tmp/input.mp4")

    args = decoder_popen.call_args[0][0]
    assert "/tmp/input.mp4" in args


def test__video_decoder_returns_popen_instance(decoder_popen: MagicMock) -> None:
    assert _video_decoder("/tmp/input.mp4") is decoder_popen.return_value


def test__video_decoder_opens_stdout_pipe(decoder_popen: MagicMock) -> None:
    _video_decoder("/tmp/input.mp4")

    assert decoder_popen.call_args[1]["stdout"] == subprocess.PIPE


def test__video_decoder_outputs_rgb24(decoder_popen: MagicMock) -> None:
    _video_decoder("/tmp/input.mp4")

    args = decoder_popen.call_args[0][0]
    assert "rgb24" in args


def test_extract_video_info_counts_packets_when_header_has_no_frame_count(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_probe(*args: str) -> str:
        return "60" if "-count_packets" in args else "1280,720,30/1,N/A"

    monkeypatch.setattr("processing.ffmpeg._run_ffprobe", fake_probe)

    assert _extract_video_info("v.webm") == (1280, 720, 30.0, 60)


@pytest.mark.parametrize(
    "stream_out,format_out,expected",
    [
        ("10.0", "99.0", 10.0),  # stream duration preferred
        ("N/A", "2.5", 2.5),  # falls back to format duration
    ],
)
def test_probe_duration_s_uses_header_duration(
    monkeypatch: pytest.MonkeyPatch, stream_out: str, format_out: str, expected: float
) -> None:
    def fake_probe(video_path: str, *args: str) -> str:
        return format_out if "format=duration" in args else stream_out

    monkeypatch.setattr("processing.ffmpeg._run_ffprobe", fake_probe)

    assert _probe_duration_s("v.webm") == expected


def test_probe_duration_s_falls_back_to_frames_over_fps(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr("processing.ffmpeg._run_ffprobe", lambda *a: "N/A")
    monkeypatch.setattr(
        "processing.ffmpeg._extract_video_info", lambda p: (1280, 720, 30.0, 60)
    )

    assert _probe_duration_s("v.webm") == 2.0


def test_kill_on_deadline_kills_on_cancel_event_without_marking_timed_out() -> None:
    proc, cancel_event = running_proc(), Event()

    with kill_on_deadline([proc], 60, cancel_event) as timed_out:
        cancel_event.set()
        assert proc.killed.wait(timeout=2)

    assert not timed_out.is_set()
    proc.kill.assert_called_once()


def test_kill_on_deadline_kills_and_marks_timed_out_when_deadline_passes() -> None:
    proc = running_proc()

    with kill_on_deadline([proc], 0) as timed_out:
        assert proc.killed.wait(timeout=2)

    assert timed_out.is_set()
    proc.kill.assert_called_once()


def test_kill_on_deadline_leaves_process_alone_when_exit_before_deadline() -> None:
    proc = running_proc()
    proc.poll.side_effect = None
    proc.poll.return_value = 0

    with kill_on_deadline([proc], 60) as timed_out:
        pass

    assert not timed_out.is_set()
    proc.kill.assert_not_called()
    proc.wait.assert_called_once()
    proc.stdout.close.assert_called_once()


def test_kill_on_deadline_kills_and_cleans_up_process_when_body_raises() -> None:
    proc = running_proc()

    with pytest.raises(RuntimeError, match="boom"):
        with kill_on_deadline([proc], 60):
            raise RuntimeError("boom")

    proc.kill.assert_called_once()
    proc.wait.assert_called_once()
    proc.stdout.close.assert_called_once()
