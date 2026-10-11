import time
from contextlib import contextmanager
from subprocess import Popen
from threading import Event, Thread
from typing import Generator, Sequence


@contextmanager
def terminate_on_deadline(
    procs: Sequence[Popen[str] | Popen[bytes]],
    timeout_s: float,
    cancel_event: Event | None = None,
) -> Generator[Event, None, None]:
    """Terminates proc if timeout_s elapses or cancel_event is set"""
    watcher_stop, timed_out = Event(), Event()
    deadline = time.monotonic() + timeout_s

    def kill_all() -> None:
        for proc in procs:
            if proc.poll() is None:
                proc.kill()

    def watch_and_kill() -> None:
        while not watcher_stop.wait(0.1):
            if cancel_event is not None and cancel_event.is_set():
                kill_all()
                return
            if time.monotonic() > deadline:
                timed_out.set()
                kill_all()
                return

    watcher = Thread(target=watch_and_kill, daemon=True)
    watcher.start()
    try:
        yield timed_out
    finally:
        watcher_stop.set()
        watcher.join()
        kill_all()
        for proc in procs:
            proc.wait()
            if proc.stdout:
                proc.stdout.close()
