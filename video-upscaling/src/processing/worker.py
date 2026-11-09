from queue import Queue
from subprocess import Popen
from threading import Event
from typing import Optional


def encoder_worker(
    encode_queue: Queue[Optional[bytes]],
    encoder: Popen[bytes],
    encoder_fail_event: Event,
) -> None:
    """
    runs in a background thread. pulls upscaled frames from encode_queue
    and writes them to ffmpeg encoder's stdin for further processing

    Args:
        encode_queue: the queue to pull upscaled frames from to write to encoder
        encoder: the encoder to write the upscaled frames to
        encoder_fail_event: threading event set whenever encoder write fails or wait is nonzero to
        signal and error
    """
    failed = False
    while True:
        frame = encode_queue.get()
        if frame is None:
            break
        if failed:
            continue
        try:
            if encoder.stdin:
                encoder.stdin.write(frame)
        except Exception:
            encoder_fail_event.set()
            failed = True
    try:
        if encoder.stdin:
            encoder.stdin.close()
    except BrokenPipeError:
        encoder_fail_event.set()
        failed = True

    if encoder.wait() != 0:  # exit status of 0 is success, fail otherwise
        encoder_fail_event.set()
