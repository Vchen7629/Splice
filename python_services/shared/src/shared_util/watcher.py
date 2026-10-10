import time
from contextlib import contextmanager
from subprocess import Popen
from threading import Event, Thread
from typing import Generator


@contextmanager
def terminate_on_deadline(
    proc: Popen[str], timeout_s: float, cancel_event: Event | None = None
) -> Generator[Event, None, None]:
    """Terminates proc if timeout_s elapses or cancel_event is set"""
    watcher_stop, timed_out = Event(), Event()
    deadline = time.monotonic() + timeout_s

    def watch_and_terminate() -> None:
        while not watcher_stop.wait(0.1):
            if cancel_event is not None and cancel_event.is_set():
                if proc.poll() is None:
                    proc.terminate()
                return
            if time.monotonic() > deadline:
                timed_out.set()
                if proc.poll() is None:
                    proc.terminate()
                return

    watcher = Thread(target=watch_and_terminate, daemon=True)
    watcher.start()
    try:
        yield timed_out
    finally:
        watcher_stop.set()
        watcher.join()
        if proc.poll() is None:
            proc.kill()
        proc.wait()
        if proc.stdout:
            proc.stdout.close()
