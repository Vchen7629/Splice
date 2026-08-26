from pathlib import Path
import pytest
import requests
import subprocess

TEST_VIDEO = Path(__file__).parent.parent / "fixtures" / "testvideo.mp4"


@pytest.fixture(scope="session")
def uploaded_test_video(
    seaweedfs_url: str, tmp_path_factory: pytest.TempPathFactory
) -> str:
    """Generates a tiny 1-frame mp4, uploads to SeaweedFS, returns the storage URL."""
    tiny = tmp_path_factory.mktemp("video") / "tiny.mp4"
    subprocess.run(
        [
            "ffmpeg",
            "-y",
            "-f",
            "lavfi",
            "-i",
            "color=c=blue:size=128x72:rate=1",
            "-frames:v",
            "1",
            str(tiny),
        ],
        check=True,
        stderr=subprocess.DEVNULL,
    )
    storage_url = f"{seaweedfs_url}/test-job/tiny.mp4"
    with open(tiny, "rb") as f:
        requests.put(
            storage_url,
            data=f,
            headers={"Content-Type": "application/octet-stream"},
            timeout=10,
        ).raise_for_status()
    return storage_url
