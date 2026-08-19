from queue import Queue
from typing import Optional
from subprocess import Popen


def encode_worker(encode_queue: Queue[Optional[bytes]], encoder: Popen[bytes]) -> None:
    """
    runs in a background thread. pulls upscaled frames from encode_queue
    and writes them to ffmpeg encoder's stdin for further processing

    Args:
        encode_queue: the queue to pull upscaled frames from to write to encoder
        encoder: the encoder to write the upscaled frames to
    """
    while True:
        frame = encode_queue.get()
        if frame is None:
            break

        if encoder.stdin:
            encoder.stdin.write(frame)

    if encoder.stdin:
        encoder.stdin.close()

    encoder.wait()
