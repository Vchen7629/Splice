from typing import Optional
from enum import IntEnum
from pathlib import Path

_BASE = Path(__file__).parent.parent


class Resolution(IntEnum):
    R_280P = 280
    R_360P = 360
    R_480P = 480
    R_740P = 740
    R_960P = 960
    R_1080P = 1080
    R_2080P = 2080

    @classmethod
    def from_string(cls, s: str) -> "Resolution":
        return cls(int(s.rstrip("p")))


def select_model(source_res: str, target_res: str) -> Optional[tuple[Path, int]]:
    """"""
    src = Resolution.from_string(source_res)
    tgt = Resolution.from_string(target_res)

    ratio = tgt / src

    if ratio <= 1:
        return None

    if ratio >= 4:
        model_path = _BASE / "weights" / "realesr-animevideov3.pth"
        resolution_scale = 4

        return model_path, resolution_scale
    else:
        model_path = _BASE / "weights" / "RealESRGANv2-animevideo-xsx2.pth"
        resolution_scale = 2

        return model_path, resolution_scale
