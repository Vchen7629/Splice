from pathlib import Path

from processing.ffmpeg import _extract_video_info, _video_decoder, _video_encoder
from tests.fixtures.processing import TEST_VIDEO


def test_extract_video_info_returns_correct_info() -> None:
    w, h, fps, nb_frames = _extract_video_info(str(TEST_VIDEO))

    assert w == 1280
    assert h == 720
    assert abs(fps - 23.976) < 0.01
    assert nb_frames == 360


def test__video_decoder_reads_correct_frame_size(one_frame_video: Path) -> None:
    w, h, _, _ = _extract_video_info(str(one_frame_video))
    frame_bytes = w * h * 3  # rgb24

    decoder = _video_decoder(str(one_frame_video))
    assert decoder.stdout is not None

    raw = decoder.stdout.read(frame_bytes)
    decoder.stdout.close()
    decoder.wait()

    assert len(raw) == frame_bytes


def test__video_decoder_returns_non_empty_frame(one_frame_video: Path) -> None:
    w, h, _, _ = _extract_video_info(str(one_frame_video))
    frame_bytes = w * h * 3

    decoder = _video_decoder(str(one_frame_video))
    assert decoder.stdout is not None

    raw = decoder.stdout.read(frame_bytes)
    decoder.stdout.close()
    decoder.wait()

    assert any(b != 0 for b in raw)


def test__video_encoder_scales_to_target_resolution(tmp_path: Path) -> None:
    in_w, in_h = 1280, 720
    output = tmp_path / "encoded.mp4"

    encoder = _video_encoder(24.0, in_w, in_h, str(output), "480p")
    assert encoder.stdin is not None
    encoder.stdin.write(bytes(in_w * in_h * 3 // 2))  # one yuv420p frame
    encoder.stdin.close()
    assert encoder.wait() == 0

    _, out_h, _, _ = _extract_video_info(str(output))
    assert out_h == 480
