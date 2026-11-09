import subprocess
from pathlib import Path
from threading import Event

import pytest

from core import output_dir
from processing.ffmpeg import _extract_video_info
from processing.video import recombine_video_audio, video_downscale, video_upscale
from tests.fixtures.processing import TEST_VIDEO, WEIGHTS_DIR, requires_cuda

# video_upscale always scales the encoder output to target_res
UPSCALE_CASES = [
    ("realesr-animevideov3.pth", 4, "2160p", 2160),
    ("RealESRGANv2-animevideo-xsx2.pth", 2, "1440p", 1440),
]


@pytest.fixture()
def recombined_video(one_frame_video: Path, tmp_path: Path) -> Path:
    """Place 1-frame clip at the job-scoped noaudio path, recombine with original audio."""
    noaudio = Path(f"{output_dir('job_id1')}/upscaled_noaudio.mp4")
    noaudio.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["ffmpeg", "-y", "-i", str(one_frame_video), str(noaudio)],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    output = tmp_path / "recombined.mp4"
    recombine_video_audio("job_id1", str(TEST_VIDEO), str(output))
    return output


@pytest.mark.parametrize("target_res", ["480p", "360p"])
def test_video_downscale_produces_output_file(target_res: str, tmp_path: Path) -> None:
    output = str(tmp_path / f"out_{target_res}.mp4")
    video_downscale(Event(), str(TEST_VIDEO), target_res, output)

    assert Path(output).exists()
    assert Path(output).stat().st_size > 0


@pytest.mark.parametrize("target_res,expected_h", [("480p", 480), ("360p", 360)])
def test_video_downscale_output_has_correct_resolution(
    target_res: str, expected_h: int, tmp_path: Path
) -> None:
    output = str(tmp_path / f"out_{target_res}.mp4")
    video_downscale(Event(), str(TEST_VIDEO), target_res, output)

    _, h, _, _ = _extract_video_info(output)

    assert h == expected_h


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
@pytest.mark.parametrize("filename,scale,target_res,expected_h", UPSCALE_CASES)
def test_video_upscale_produces_output_file(
    one_frame_video: Path, filename: str, scale: int, target_res: str, expected_h: int
) -> None:
    output = Path(f"{output_dir('jobid1')}/upscaled_noaudio.mp4")
    output.parent.mkdir(parents=True, exist_ok=True)

    video_upscale(
        Event(),
        "jobid1",
        str(one_frame_video),
        WEIGHTS_DIR / filename,
        scale,
        target_res,
    )

    assert output.exists()
    assert output.stat().st_size > 0


@requires_cuda
@pytest.mark.parametrize("filename,scale,target_res,expected_h", UPSCALE_CASES)
def test_video_upscale_output_has_target_resolution(
    one_frame_video: Path, filename: str, scale: int, target_res: str, expected_h: int
) -> None:
    output = Path(f"{output_dir('job_id1')}/upscaled_noaudio.mp4")
    output.parent.mkdir(parents=True, exist_ok=True)

    video_upscale(
        Event(),
        "job_id1",
        str(one_frame_video),
        WEIGHTS_DIR / filename,
        scale,
        target_res,
    )

    _, out_h, _, _ = _extract_video_info(str(output))
    assert out_h == expected_h
