from enum import IntEnum
from pathlib import Path
from typing import Optional

from realesrgan import RealESRGANer
from realesrgan.archs.srvgg_arch import SRVGGNetCompact
from torch import compile

_BASE = Path(__file__).parent.parent


class Resolution(IntEnum):
    R_280P = 280
    R_360P = 360
    R_480P = 480
    R_720P = 720
    R_960P = 960
    R_1080P = 1080
    R_1440P = 1440
    R_2160P = 2160

    @classmethod
    def from_string(cls, s: str) -> "Resolution":
        s = s.strip().lower()
        h = 2160 if s == "4k" else int(s.rstrip("p"))

        return min(cls, key=lambda r: abs(r.value - h))  # pyrefly: ignore[implicit-any-lambda]


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


def _load_model(model_path: Path, scale: int) -> RealESRGANer:
    """
    Loads a RealESRGAN upscaler model onto CUDA with fp16 and torch.compile optimization.
    Tiling is disabled so full-frame inference only.

    Args:
        model_path: the path to the model weights
        scale: the upscaling ratio for the model to use

    Returns:
        The loaded RealESRGANer model

    Raises:
        ValueError if the scale isnt 2 or 4 since we only support x2 and x4 upscaling
    """
    if scale not in (2, 4):
        raise ValueError("scale must be either 2 or 4")

    model = SRVGGNetCompact(
        num_in_ch=3,
        num_out_ch=3,
        num_feat=64,
        num_conv=16,
        upscale=scale,
        act_type="prelu",
    )

    upsampler = RealESRGANer(
        scale=scale,
        model_path=str(model_path),
        model=model,
        tile=0,
        tile_pad=10,
        pre_pad=0,
        device="cuda",
    )
    upsampler.model = upsampler.model.half()
    upsampler.model = compile(upsampler.model, mode="reduce-overhead")
    return upsampler
