from threading import Event
from unittest.mock import MagicMock

import pytest

from shared_util import terminate_on_deadline


def running_proc() -> MagicMock:
    proc = MagicMock()
    proc.poll.return_value = None
    proc.terminated = Event()
    proc.terminate.side_effect = proc.terminated.set
    return proc


def test_terminates_on_cancel_event_without_marking_timed_out() -> None:
    proc, cancel_event = running_proc(), Event()

    with terminate_on_deadline(proc, 60, cancel_event) as timed_out:
        cancel_event.set()
        assert proc.terminated.wait(timeout=2)

    assert not timed_out.is_set()
    proc.terminate.assert_called_once


def test_terminates_and_marks_timed_out_when_deadline_passes() -> None:
    proc = running_proc()

    with terminate_on_deadline(proc, 0) as timed_out:
        assert proc.terminated.wait(timeout=2)

    assert timed_out.is_set()
    proc.terminate.assert_called_once()


def test_leaves_process_alone_when_exit_before_deadline() -> None:
    proc = running_proc()
    proc.poll.return_value = 0

    with terminate_on_deadline(proc, 60) as timed_out:
        pass

    assert not timed_out.is_set()
    proc.terminate.assert_not_called()
    proc.kill.assert_not_called()
    proc.wait.assert_called_once()
    proc.stdout.close.assert_called_once()


def test_kills_and_cleans_up_process_when_body_raises() -> None:
    proc = running_proc()

    with pytest.raises(RuntimeError, match="boom"):
        with terminate_on_deadline(proc, 60):
            raise RuntimeError("boom")

    proc.terminate.assert_not_called()
    proc.kill.assert_called_once()
    proc.wait.assert_called_once()
    proc.stdout.close.assert_called_once()
