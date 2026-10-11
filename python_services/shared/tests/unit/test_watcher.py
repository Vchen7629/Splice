from threading import Event
from unittest.mock import MagicMock

import pytest

from shared_util import kill_on_deadline


def running_proc() -> MagicMock:
    proc = MagicMock()
    proc.killed = Event()
    proc.poll.side_effect = lambda: -9 if proc.killed.is_set() else None
    proc.kill.side_effect = proc.killed.set
    return proc


def test_kills_on_cancel_event_without_marking_timed_out() -> None:
    proc, cancel_event = running_proc(), Event()

    with kill_on_deadline([proc], 60, cancel_event) as timed_out:
        cancel_event.set()
        assert proc.killed.wait(timeout=2)

    assert not timed_out.is_set()
    proc.kill.assert_called_once()


def test_kills_and_marks_timed_out_when_deadline_passes() -> None:
    proc = running_proc()

    with kill_on_deadline([proc], 0) as timed_out:
        assert proc.killed.wait(timeout=2)

    assert timed_out.is_set()
    proc.kill.assert_called_once()


def test_leaves_process_alone_when_exit_before_deadline() -> None:
    proc = running_proc()
    proc.poll.side_effect = None
    proc.poll.return_value = 0

    with kill_on_deadline([proc], 60) as timed_out:
        pass

    assert not timed_out.is_set()
    proc.kill.assert_not_called()
    proc.wait.assert_called_once()
    proc.stdout.close.assert_called_once()


def test_kills_and_cleans_up_process_when_body_raises() -> None:
    proc = running_proc()

    with pytest.raises(RuntimeError, match="boom"):
        with kill_on_deadline([proc], 60):
            raise RuntimeError("boom")

    proc.kill.assert_called_once()
    proc.wait.assert_called_once()
    proc.stdout.close.assert_called_once()
