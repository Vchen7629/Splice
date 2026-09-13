from queue import Queue
from threading import Event
from typing import Optional
from unittest.mock import MagicMock

from src.processing.worker import encoder_worker


def _make_encoder(has_stdin: bool = True) -> MagicMock:
    encoder = MagicMock()
    encoder.stdin = MagicMock() if has_stdin else None
    return encoder


def _run(
    frames: list[Optional[bytes]],
    encoder: MagicMock,
    fail_event: Optional[MagicMock] = None,
) -> None:
    if fail_event is None:
        fail_event = MagicMock(spec=Event)

    q: Queue[Optional[bytes]] = Queue()
    for f in frames:
        q.put(f)
    q.put(None)

    encoder_worker(q, encoder, fail_event)


def test_writes_each_frame_to_encoder_stdin() -> None:
    encoder = _make_encoder()
    _run([b"frame1", b"frame2", b"frame3"], encoder)

    assert encoder.stdin.write.call_count == 3


def test_writes_frames_in_order() -> None:
    encoder = _make_encoder()
    _run([b"first", b"second"], encoder)

    calls = [c.args[0] for c in encoder.stdin.write.call_args_list]
    assert calls == [b"first", b"second"]


def test_does_not_write_none_sentinel_to_stdin() -> None:
    encoder = _make_encoder()
    _run([b"frame"], encoder)

    written = [c.args[0] for c in encoder.stdin.write.call_args_list]
    assert None not in written


def test_no_writes_when_queue_only_has_sentinel() -> None:
    encoder = _make_encoder()
    _run([], encoder)

    encoder.stdin.write.assert_not_called()


def test_closes_stdin_after_sentinel() -> None:
    encoder = _make_encoder()
    _run([b"frame"], encoder)

    encoder.stdin.close.assert_called_once()


def test_calls_wait_after_closing_stdin() -> None:
    encoder = _make_encoder()
    _run([b"frame"], encoder)

    encoder.wait.assert_called_once()


def test_does_not_write_when_stdin_is_none() -> None:
    encoder = _make_encoder(has_stdin=False)
    _run([b"frame1", b"frame2"], encoder)  # should not raise


def test_does_not_close_stdin_when_stdin_is_none() -> None:
    encoder = _make_encoder(has_stdin=False)
    _run([b"frame"], encoder)

    assert encoder.stdin is None  # no close attempted, no AttributeError


def test_sets_event_when_write_fails() -> None:
    encoder = _make_encoder()
    encoder.stdin.write.side_effect = OSError("broken pipe")
    encoder.wait.return_value = 0
    fail_event = MagicMock(spec=Event)

    _run([b"frame"], encoder, fail_event)

    fail_event.set.assert_called_once()


def test_sets_event_when_wait_nonzero_return_val() -> None:
    encoder = _make_encoder()
    encoder.wait.return_value = 999
    fail_event = MagicMock(spec=Event)

    _run([b"frame"], encoder, fail_event)

    fail_event.set.assert_called_once()


def test_drains_remaining_frames_after_first_failure_instead_of_leaving_them_stuck() -> (
    None
):
    encoder = _make_encoder()
    encoder.stdin.write.side_effect = OSError("broken pipe")
    encoder.wait.return_value = 0
    fail_event = MagicMock(spec=Event)

    frames = [b"frame1", b"frame2", b"frame3"]
    q: Queue[Optional[bytes]] = Queue(maxsize=len(frames) + 1)
    for f in frames:
        q.put(f)
    q.put(None)

    encoder_worker(q, encoder, fail_event)

    encoder.stdin.write.assert_called_once_with(b"frame1")
    assert q.qsize() == 0


def test_sets_event_when_encoder_close_raises() -> None:
    encoder = _make_encoder()
    encoder.stdin.close.side_effect = BrokenPipeError("some broken pipe")
    encoder.wait.return_value = 0
    fail_event = MagicMock(spec=Event)

    _run([b"frame"], encoder, fail_event)

    fail_event.set.assert_called_once()
