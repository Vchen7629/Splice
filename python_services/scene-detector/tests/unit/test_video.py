import os
import tempfile
from threading import Event
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest
from shared_handler.exceptions import JobCancelledError

from src.processing.video import split_into_chunks

MOCK_CANCEL_EVENT = MagicMock(spec=Event)
MOCK_CANCEL_EVENT.is_set.return_value = False


class FakeTimecode:
    """minimal stand-in for scenedetect's FrameTimecode: supports subtraction and
    get_seconds(), which is all split_into_chunks needs from a scene boundary"""

    def __init__(self, seconds: float) -> None:
        self.seconds = seconds

    def get_seconds(self) -> float:
        return self.seconds

    def __sub__(self, other: "FakeTimecode") -> "FakeTimecode":
        return FakeTimecode(self.seconds - other.seconds)


def scene_manager_stopping_immediately(scenes: list) -> MagicMock:
    """a mock SceneManager whose detect_scenes() ends the loop on the first call"""
    manager = MagicMock()
    manager.detect_scenes.return_value = 0
    manager.get_scene_list.return_value = scenes
    return manager


def fake_popen(progress_lines: tuple[str, ...] = ()) -> MagicMock:
    """a Popen replacement whose process streams `progress_lines` then exits 0"""
    popen = MagicMock()
    proc = popen.return_value
    proc.stdout = iter(progress_lines)
    proc.wait.return_value = 0
    proc.poll.return_value = 0
    return popen


@pytest.fixture
def detection():
    video = MagicMock(frame_rate=30, duration=None)
    manager = scene_manager_stopping_immediately([])
    with (
        patch("src.processing.video.open_video", return_value=video),
        patch("src.processing.video.SceneManager", return_value=manager),
    ):
        yield SimpleNamespace(video=video, manager=manager)


def test_returns_correct_chunk_paths(detection) -> None:
    """Returns zero-padded scene paths based on detected scene count"""
    detection.manager.get_scene_list.return_value = [
        (FakeTimecode(0), FakeTimecode(1))
    ] * 3

    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch("src.processing.video.subprocess.Popen", new=fake_popen()) as popen,
            patch(
                "src.processing.video.glob.glob",
                return_value=["chunk-1", "chunk-2", "chunk-3"],
            ) as mock_glob,
        ):
            result = split_into_chunks(
                MOCK_CANCEL_EVENT, "/videos/myvideo.mp4", output_dir
            )

    assert result == ["chunk-1", "chunk-2", "chunk-3"]
    assert popen.call_args.args[0][-1] == os.path.join(
        output_dir, "myvideo-Scene-%03d.mp4"
    )
    mock_glob.assert_called_once_with(os.path.join(output_dir, "myvideo-Scene-*.mp4"))


def test_no_scene_boundaries_copies_original_as_single_chunk(detection) -> None:
    """When no scene boundaries are detected the original file is returned as one chunk,
    and on_progress(100) is still called on this fallback path."""
    with (
        tempfile.TemporaryDirectory() as src_dir,
        tempfile.TemporaryDirectory() as output_dir,
    ):
        src = os.path.join(src_dir, "myvideo.mp4")
        src_bytes = b"fake-video-bytes"
        with open(src, "wb") as f:
            f.write(src_bytes)

        percents: list[int] = []
        result = split_into_chunks(
            MOCK_CANCEL_EVENT,
            src,
            output_dir,
            on_progress=percents.append,
        )

        expected_output = os.path.join(output_dir, "myvideo.mp4")
        assert result == [expected_output]
        assert os.path.exists(expected_output)
        with open(expected_output, "rb") as f:
            assert f.read() == src_bytes
        assert percents == [100]


def test_progress_capped_at_90_during_detection_then_reaches_100_after_split(
    detection,
) -> None:
    fake_video = detection.video
    fake_video.duration = SimpleNamespace(frame_num=300, get_seconds=lambda: 10)
    fake_video.frame_number = 0

    def fake_detect_scenes(video: object, duration: int) -> int:
        if detection.video.frame_number >= 300:
            return 0
        detection.video.frame_number = min(300, detection.video.frame_number + 150)
        return 150

    detection.manager.detect_scenes.side_effect = fake_detect_scenes
    scene = (FakeTimecode(0), FakeTimecode(1))
    detection.manager.get_scene_list.return_value = [scene, scene]

    percents: list[int] = []
    with (
        patch(
            "src.processing.video.subprocess.Popen",
            new=fake_popen(("out_time_us=5000000\n", "out_time_us=10000000\n")),
        ),
        patch("src.processing.video.glob.glob", return_value=["a", "b"]),
    ):
        with tempfile.TemporaryDirectory() as output_dir:
            split_into_chunks(
                MOCK_CANCEL_EVENT,
                "/videos/myvideo.mp4",
                output_dir,
                on_progress=percents.append,
            )

    detect_phase, split_phase = percents[:2], percents[2:]
    assert all(p <= 90 for p in detect_phase)
    assert percents == sorted(percents)
    assert split_phase[-1] == 100


def test_raises_job_cancelled_when_cancel_event_is_set_during_detect_scan(
    detection,
) -> None:
    detection.manager.detect_scenes.return_value = 150
    cancel_event = MagicMock(spec=Event)
    cancel_event.is_set.return_value = True

    with tempfile.TemporaryDirectory() as output_dir:
        with pytest.raises(
            JobCancelledError,
            match="split_into_chunks cancelled during detect scan",
        ):
            split_into_chunks(cancel_event, "/videos/myvideo.mp4", output_dir)


def test_raises_job_cancelled_and_terminates_ffmpeg_when_cancelled_mid_split(
    detection,
) -> None:
    detection.manager.get_scene_list.return_value = [
        (FakeTimecode(0), FakeTimecode(1))
    ] * 3
    cancel_event = MagicMock(spec=Event)
    cancel_event.is_set.side_effect = [False, True]
    popen = fake_popen(("frame=1\n",))

    with tempfile.TemporaryDirectory() as output_dir:
        with patch("src.processing.video.subprocess.Popen", new=popen):
            with pytest.raises(
                JobCancelledError,
                match="split_into_chunks cancelled during scene-split",
            ):
                split_into_chunks(cancel_event, "/videos/myvideo.mp4", output_dir)

    popen.return_value.terminate.assert_called_once()


def test_raises_job_cancelled_when_set_after_detect_scan_with_no_scenes(
    detection,
) -> None:
    cancel_event = MagicMock(spec=Event)
    cancel_event.is_set.return_value = True

    with tempfile.TemporaryDirectory() as output_dir:
        with patch("src.processing.video.shutil.copy2") as mock_copy2:
            with pytest.raises(
                JobCancelledError,
                match="split_into_chunks cancelled after detect scan",
            ):
                split_into_chunks(cancel_event, "/videos/myvideo.mp4", output_dir)

    detection.manager.get_scene_list.assert_not_called()
    mock_copy2.assert_not_called()


def test_raises_when_chunk_count_does_not_match_scene_count(detection) -> None:
    detection.manager.get_scene_list.return_value = [
        (FakeTimecode(0), FakeTimecode(1)),
        (FakeTimecode(1), FakeTimecode(2)),
    ]

    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch("src.processing.video.subprocess.Popen", new=fake_popen()),
            patch("src.processing.video.glob.glob", return_value=["only-one"]),
        ):
            with pytest.raises(RuntimeError, match="expected 2 scene chunks"):
                split_into_chunks(MOCK_CANCEL_EVENT, "/videos/myvideo.mp4", output_dir)


def test_ffmpeg_command_uses_same_cut_points_for_keyframes_and_segments(
    detection,
) -> None:
    detection.manager.get_scene_list.return_value = [
        (FakeTimecode(0), FakeTimecode(2.5)),
        (FakeTimecode(2.5), FakeTimecode(7)),
        (FakeTimecode(7), FakeTimecode(9)),
    ]

    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch(
                "src.processing.video.subprocess.Popen", new=fake_popen()
            ) as mock_popen,
            patch("src.processing.video.glob.glob", return_value=["a", "b", "c"]),
        ):
            split_into_chunks(MOCK_CANCEL_EVENT, "/videos/myvideo.mp4", output_dir)

    cmd = mock_popen.call_args.args[0]
    assert cmd[cmd.index("-force_key_frames") + 1] == "2.5,7"
    assert cmd[cmd.index("-segment_times") + 1] == "2.5,7"
    assert "-sn" in cmd
