from queue import Queue
from typing import Optional
from unittest.mock import MagicMock
from src.processing.worker import encode_worker


def _make_encoder(has_stdin: bool = True) -> MagicMock:
    encoder = MagicMock()
    encoder.stdin = MagicMock() if has_stdin else None
    return encoder


def _run(frames: list[Optional[bytes]], encoder: MagicMock) -> None:
    q: Queue[Optional[bytes]] = Queue()
    for f in frames:
        q.put(f)
    q.put(None)
    encode_worker(q, encoder)


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
