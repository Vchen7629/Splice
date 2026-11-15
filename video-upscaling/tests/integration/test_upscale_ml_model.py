import pytest
import torch

from processing.upscale_ml_model import _load_model
from tests.fixtures.processing import WEIGHTS_DIR, requires_cuda

MODELS = [
    ("realesr-animevideov3.pth", 4),
    ("RealESRGANv2-animevideo-xsx2.pth", 2),
]


@requires_cuda
@pytest.mark.parametrize("filename,scale", MODELS)
def test_load_model_is_cuda_fp16_eval(filename: str, scale: int) -> None:
    result = _load_model(WEIGHTS_DIR / filename, scale=scale)

    param = next(result.parameters())
    assert param.device.type == "cuda"
    assert param.dtype == torch.float16
    assert result.training is False


@requires_cuda
@pytest.mark.parametrize("filename,scale", MODELS)
def test_load_model_output_matches_scale(filename: str, scale: int) -> None:
    result = _load_model(WEIGHTS_DIR / filename, scale=scale)
    batch = torch.rand(1, 3, 16, 24, device="cuda", dtype=torch.float16)

    with torch.inference_mode():
        out = result(batch)

    assert out.shape == (1, 3, 16 * scale, 24 * scale)
