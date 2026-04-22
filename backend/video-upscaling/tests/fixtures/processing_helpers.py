from pathlib import Path
from typing import Any
from typing import Generator
from unittest.mock import patch
from unittest.mock import MagicMock
from processing.video import recombine_video_audio
import pytest
import subprocess
import numpy as np


TEST_VIDEO = Path(__file__).parent / "testvideo.mp4"


@pytest.fixture(scope="module")
def one_frame_video(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """Extract a single frame from testvideo as a 1-frame mp4."""
    out = tmp_path_factory.mktemp("frames") / "one_frame.mp4"
    subprocess.run(
        ["ffmpeg", "-y", "-i", str(TEST_VIDEO), "-frames:v", "1", str(out)],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    return out


@pytest.fixture()
def recombined_video(one_frame_video: Path, tmp_path: Path) -> Path:
    """Place 1-frame clip at the hardcoded noaudio path, recombine with original audio."""
    subprocess.run(
        ["ffmpeg", "-y", "-i", str(one_frame_video), "/tmp/upscaled_noaudio.mp4"],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    output = tmp_path / "recombined.mp4"
    recombine_video_audio(str(TEST_VIDEO), str(output))
    return output


def make_fake_decoder(frames: list[np.ndarray]) -> MagicMock:
    """Build a mock decoder whose stdout yields the given raw bgr frames then EOF."""
    stdout = MagicMock()
    stdout.read.side_effect = [f.tobytes() for f in frames] + [b""]
    decoder = MagicMock()
    decoder.stdout = stdout
    return decoder


@pytest.fixture
def mock_deps() -> Generator[dict[str, Any], Any, None]:
    mock_net = MagicMock()
    mock_net.half.return_value = mock_net

    mock_upsampler = MagicMock()
    mock_upsampler.model = mock_net

    mock_compiled = MagicMock()

    with (
        patch(
            "src.processing.load_model.SRVGGNetCompact", return_value=mock_net
        ) as mock_srvgg,
        patch(
            "src.processing.load_model.RealESRGANer", return_value=mock_upsampler
        ) as mock_realesrgan,
        patch(
            "src.processing.load_model.torch.compile", return_value=mock_compiled
        ) as mock_compile,
    ):
        yield {
            "srvgg": mock_srvgg,
            "realesrgan": mock_realesrgan,
            "compile": mock_compile,
            "net": mock_net,
            "upsampler": mock_upsampler,
        }


@pytest.fixture
def video_upscale_patches() -> Generator[dict[str, Any], Any, None]:
    """Patches all external dependencies used by video_upscale."""
    mock_thread = MagicMock()
    mock_encoder = MagicMock()

    with (
        patch(
            "src.processing.video.extract_video_info", return_value=(64, 64, 24.0)
        ) as mock_info,
        patch("src.processing.video.load_model", return_value=MagicMock()) as mock_load,
        patch("src.processing.video.video_decoder") as mock_decoder,
        patch(
            "src.processing.video.video_encoder", return_value=mock_encoder
        ) as mock_enc,
        patch(
            "src.processing.video.flush_batch", return_value=(0.0, 0.0, 0)
        ) as mock_flush,
        patch("src.processing.video.encode_worker") as mock_worker,
        patch("src.processing.video.threading.Thread", return_value=mock_thread) as _,
        patch("src.processing.video.recombine_video_audio") as mock_recombine,
        patch("src.processing.video.settings") as mock_settings,
    ):
        mock_settings.BATCH_SIZE = 4
        yield {
            "info": mock_info,
            "load": mock_load,
            "decoder": mock_decoder,
            "encoder": mock_enc,
            "flush": mock_flush,
            "worker": mock_worker,
            "recombine": mock_recombine,
            "settings": mock_settings,
        }
