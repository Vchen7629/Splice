import subprocess
from pathlib import Path
from threading import Event
from unittest.mock import MagicMock

import pytest
import torch

from src.processing.video import (
    _extract_video_info,
    recombine_video_audio,
    video_decoder,
    video_downscale,
    video_encoder,
    video_upscale,
)
from tests.fixtures.processing_helpers import TEST_VIDEO

MOCK_CANCEL_EVENT = MagicMock(spec=Event)
MOCK_CANCEL_EVENT.is_set.return_value = False
WEIGHTS_DIR = Path(__file__).parent.parent.parent / "src" / "weights"

requires_cuda = pytest.mark.skipif(
    not torch.cuda.is_available(), reason="CUDA not available"
)


@pytest.fixture()
def recombined_video(one_frame_video: Path, tmp_path: Path) -> Path:
    """Place 1-frame clip at the job-scoped noaudio path, recombine with original audio."""
    subprocess.run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(one_frame_video),
            "/tmp/upscaled_noaudio-job_id1.mp4",
        ],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    output = tmp_path / "recombined.mp4"
    recombine_video_audio("job_id1", str(TEST_VIDEO), str(output))
    return output


def test_extract_video_info_returns_correct_info() -> None:
    w, h, fps, nb_frames = _extract_video_info(str(TEST_VIDEO))

    assert w == 1280
    assert h == 720
    assert abs(fps - 23.976) < 0.01
    assert nb_frames == 360


@pytest.mark.parametrize("target_res", ["480p", "360p"])
def test_video_downscale_produces_output_file(target_res: str, tmp_path: Path) -> None:
    output = str(tmp_path / f"out_{target_res}.mp4")
    video_downscale(MOCK_CANCEL_EVENT, str(TEST_VIDEO), target_res, output)

    assert Path(output).exists()
    assert Path(output).stat().st_size > 0


@pytest.mark.parametrize("target_res,expected_h", [("480p", 480), ("360p", 360)])
def test_video_downscale_output_has_correct_resolution(
    target_res: str, expected_h: int, tmp_path: Path
) -> None:
    output = str(tmp_path / f"out_{target_res}.mp4")
    video_downscale(MOCK_CANCEL_EVENT, str(TEST_VIDEO), target_res, output)

    _, h, _, _ = _extract_video_info(output)

    assert h == expected_h


def test_video_decoder_reads_correct_frame_size(one_frame_video: Path) -> None:
    w, h, _, _ = _extract_video_info(str(one_frame_video))
    frame_bytes = w * h * 3  # rgb24

    decoder = video_decoder(str(one_frame_video))
    assert decoder.stdout is not None

    raw = decoder.stdout.read(frame_bytes)
    decoder.stdout.close()
    decoder.wait()

    assert len(raw) == frame_bytes


def test_video_decoder_returns_non_empty_frame(one_frame_video: Path) -> None:
    w, h, _, _ = _extract_video_info(str(one_frame_video))
    frame_bytes = w * h * 3

    decoder = video_decoder(str(one_frame_video))
    assert decoder.stdout is not None

    raw = decoder.stdout.read(frame_bytes)
    decoder.stdout.close()
    decoder.wait()

    assert any(b != 0 for b in raw)


def test_video_encoder_scales_to_target_resolution(tmp_path: Path) -> None:
    in_w, in_h = 1280, 720
    output = tmp_path / "encoded.mp4"

    encoder = video_encoder(24.0, in_w, in_h, str(output), "480p")
    assert encoder.stdin is not None
    encoder.stdin.write(bytes(in_w * in_h * 3 // 2))  # one yuv420p frame
    encoder.stdin.close()
    assert encoder.wait() == 0

    _, out_h, _, _ = _extract_video_info(str(output))
    assert out_h == 480


def test_recombine_video_audio_produces_output_file(recombined_video: Path) -> None:
    assert recombined_video.exists()
    assert recombined_video.stat().st_size > 0


def test_recombine_video_audio_output_has_audio_stream(recombined_video: Path) -> None:
    probe = subprocess.run(
        [
            "ffprobe",
            "-v",
            "error",
            "-select_streams",
            "a",
            "-show_entries",
            "stream=codec_type",
            "-of",
            "csv=p=0",
            str(recombined_video),
        ],
        capture_output=True,
        text=True,
    )
    assert "audio" in probe.stdout


@requires_cuda
@pytest.mark.parametrize(
    "filename,scale",
    [
        ("realesr-animevideov3.pth", 4),
        ("RealESRGANv2-animevideo-xsx2.pth", 2),
    ],
)
def test_video_upscale_produces_output_file(
    one_frame_video: Path, filename: str, scale: int
) -> None:
    output = Path("/tmp/upscaled_noaudio-jobid1.mp4")
    output.unlink(missing_ok=True)
    video_upscale(
        MOCK_CANCEL_EVENT, "jobid1", str(one_frame_video), WEIGHTS_DIR / filename, scale
    )

    assert output.exists()
    assert output.stat().st_size > 0


@requires_cuda
@pytest.mark.parametrize(
    "filename,scale",
    [
        ("realesr-animevideov3.pth", 4),
        ("RealESRGANv2-animevideo-xsx2.pth", 2),
    ],
)
def test_video_upscale_output_has_correct_resolution(
    one_frame_video: Path, filename: str, scale: int
) -> None:
    src_w, src_h, _, _ = _extract_video_info(str(one_frame_video))
    output = "/tmp/upscaled_noaudio-job_id1.mp4"
    Path(output).unlink(missing_ok=True)

    video_upscale(
        MOCK_CANCEL_EVENT,
        "job_id1",
        str(one_frame_video),
        WEIGHTS_DIR / filename,
        scale,
    )

    out_w, out_h, _, _ = _extract_video_info(output)
    assert out_w == src_w * scale
    assert out_h == src_h * scale
