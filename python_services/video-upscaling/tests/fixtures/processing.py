import subprocess
from pathlib import Path
from threading import Event
from unittest.mock import MagicMock

import pytest
import torch

TEST_VIDEO = Path(__file__).parent / "testvideo.mp4"
WEIGHTS_DIR = Path(__file__).parent.parent.parent / "src" / "weights"

requires_cuda = pytest.mark.skipif(
    not torch.cuda.is_available(), reason="CUDA not available"
)


@pytest.fixture(scope="session")
def one_frame_video(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """1-frame 1280x720 mp4 cut from testvideo (audio track kept)"""
    out = tmp_path_factory.mktemp("frames") / "one_frame.mp4"
    subprocess.run(
        ["ffmpeg", "-y", "-i", str(TEST_VIDEO), "-frames:v", "1", str(out)],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    return out


def running_proc() -> MagicMock:
    """Popen stand-in that reports running until kill(), proc.killed is set on kill"""
    proc = MagicMock()
    proc.killed = Event()
    proc.poll.side_effect = lambda: -9 if proc.killed.is_set() else None
    proc.kill.side_effect = proc.killed.set
    return proc
