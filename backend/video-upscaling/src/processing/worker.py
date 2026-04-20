from queue import Queue
from subprocess import Popen


def encode_worker(encode_queue: Queue, encoder: Popen[bytes]) -> None:
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

        encoder.stdin.write(frame)

    encoder.stdin.close()
    encoder.wait()
