from structlog.stdlib import BoundLogger
from threading import Event
from unittest.mock import patch, MagicMock
from types import SimpleNamespace
from src.processing.video import split_into_chunks
from shared_handler.exceptions import JobCancelledError
import os
import tempfile
import pytest

MOCK_CANCEL_EVENT = MagicMock(spec=Event)
MOCK_CANCEL_EVENT.is_set.return_value = False
MOCK_LOGGER = MagicMock(spec=BoundLogger)


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


def test_returns_correct_chunk_paths() -> None:
    """Returns zero-padded scene paths based on detected scene count"""
    scenes = [(FakeTimecode(0), FakeTimecode(1))] * 3
    manager = scene_manager_stopping_immediately(scenes)

    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch(
                "src.processing.video.open_video",
                return_value=MagicMock(frame_rate=30),
            ),
            patch("src.processing.video.SceneManager", return_value=manager),
            patch("src.processing.video.subprocess.run"),
        ):
            result = split_into_chunks(
                MOCK_LOGGER, MOCK_CANCEL_EVENT, "/videos/myvideo.mp4", output_dir
            )

    assert result == [
        os.path.join(output_dir, "myvideo-Scene-001.mp4"),
        os.path.join(output_dir, "myvideo-Scene-002.mp4"),
        os.path.join(output_dir, "myvideo-Scene-003.mp4"),
    ]


def test_no_scene_boundaries_copies_original_as_single_chunk() -> None:
    """When no scene boundaries are detected the original file is returned as one chunk,
    and on_progress(100) is still called on this fallback path."""
    manager = scene_manager_stopping_immediately([])

    with (
        tempfile.TemporaryDirectory() as src_dir,
        tempfile.TemporaryDirectory() as output_dir,
    ):
        src = os.path.join(src_dir, "myvideo.mp4")
        src_bytes = b"fake-video-bytes"
        with open(src, "wb") as f:
            f.write(src_bytes)

        percents: list[int] = []
        with (
            patch(
                "src.processing.video.open_video",
                return_value=MagicMock(frame_rate=30),
            ),
            patch("src.processing.video.SceneManager", return_value=manager),
        ):
            result = split_into_chunks(
                MOCK_LOGGER,
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


def test_on_progress_defaults_to_none_safely() -> None:
    """split_into_chunks works without an on_progress callback"""
    manager = scene_manager_stopping_immediately([])

    with (
        tempfile.TemporaryDirectory() as src_dir,
        tempfile.TemporaryDirectory() as output_dir,
    ):
        src = os.path.join(src_dir, "myvideo.mp4")
        with open(src, "wb") as f:
            f.write(b"fake-video-bytes")

        with (
            patch(
                "src.processing.video.open_video",
                return_value=MagicMock(frame_rate=30),
            ),
            patch("src.processing.video.SceneManager", return_value=manager),
        ):
            result = split_into_chunks(MOCK_LOGGER, MOCK_CANCEL_EVENT, src, output_dir)

    assert result == [os.path.join(output_dir, "myvideo.mp4")]


def test_progress_capped_at_90_during_detection_then_reaches_100_after_split() -> None:
    """progress stays <= 90 while detect_scenes is still running, then climbs to 100
    once splitting finishes — never decreasing across either phase."""
    fake_video = SimpleNamespace(
        duration=SimpleNamespace(frame_num=300), frame_number=0, frame_rate=30
    )

    def fake_detect_scenes(video: object, duration: int) -> int:
        if fake_video.frame_number >= 300:
            return 0
        fake_video.frame_number = min(300, fake_video.frame_number + 150)
        return 150

    manager = MagicMock()
    manager.detect_scenes.side_effect = fake_detect_scenes
    scene = (FakeTimecode(0), FakeTimecode(1))
    manager.get_scene_list.return_value = [scene, scene]

    percents: list[int] = []
    with (
        patch("src.processing.video.open_video", return_value=fake_video),
        patch("src.processing.video.SceneManager", return_value=manager),
        patch("src.processing.video.subprocess.run"),
    ):
        with tempfile.TemporaryDirectory() as output_dir:
            split_into_chunks(
                MOCK_LOGGER,
                MOCK_CANCEL_EVENT,
                "/videos/myvideo.mp4",
                output_dir,
                on_progress=percents.append,
            )

    detect_phase, split_phase = percents[:2], percents[2:]
    assert all(p <= 90 for p in detect_phase)
    assert percents == sorted(percents)
    assert split_phase[-1] == 100


def test_raises_job_cancelled_when_cancel_event_is_set_during_detect_scan() -> None:
    manager = SimpleNamespace(
        add_detector=MagicMock(),
        detect_scenes=MagicMock(return_value=150),
        get_scene_list=MagicMock(),
    )
    cancel_event = MagicMock(spec=Event)
    cancel_event.is_set.return_value = True

    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch(
                "src.processing.video.open_video",
                return_value=MagicMock(frame_rate=30, duration=None),
            ),
            patch("src.processing.video.SceneManager", return_value=manager),
            patch("src.processing.video.subprocess.run"),
        ):
            with pytest.raises(
                JobCancelledError, match="cancelled during detect scan for job"
            ):
                split_into_chunks(
                    MOCK_LOGGER, cancel_event, "/videos/myvideo.mp4", output_dir
                )


def test_raises_job_cancelled_when_cancel_event_is_set_before_scene_split() -> None:
    scenes = [(FakeTimecode(0), FakeTimecode(1))] * 3
    manager = SimpleNamespace(
        add_detector=MagicMock(),
        detect_scenes=MagicMock(return_value=0),
        get_scene_list=MagicMock(return_value=scenes),
    )
    cancel_event = MagicMock(spec=Event)
    cancel_event.is_set.side_effect = [False, True]

    with tempfile.TemporaryDirectory() as output_dir:
        with (
            patch(
                "src.processing.video.open_video",
                return_value=MagicMock(frame_rate=30, duration=None),
            ),
            patch("src.processing.video.SceneManager", return_value=manager),
            patch("src.processing.video.subprocess.run"),
        ):
            with pytest.raises(
                JobCancelledError, match="cancelled before scene 1 for job"
            ):
                split_into_chunks(
                    MOCK_LOGGER, cancel_event, "/videos/myvideo.mp4", output_dir
                )
