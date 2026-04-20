from realesrgan import RealESRGANer
from realesrgan.archs.srvgg_arch import SRVGGNetCompact
import torch


def load_model(model_path: str, scale: int) -> RealESRGANer:
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
        model_path=model_path,
        model=model,
        tile=0,
        tile_pad=10,
        pre_pad=0,
        device="cuda",
    )
    upsampler.model = upsampler.model.half()
    upsampler.model = torch.compile(upsampler.model, mode="reduce-overhead")
    return upsampler
