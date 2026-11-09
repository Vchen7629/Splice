import pytest
import torch
from realesrgan import RealESRGANer

from processing.upscale_ml_model import _load_model
from tests.fixtures.processing import WEIGHTS_DIR, requires_cuda

MODELS = [
    ("realesr-animevideov3.pth", 4),
    ("RealESRGANv2-animevideo-xsx2.pth", 2),
]


@requires_cuda
@pytest.mark.parametrize("filename,scale", MODELS)
def test_load_model_returns_realesrganer(filename: str, scale: int) -> None:
    result = _load_model(WEIGHTS_DIR / filename, scale=scale)

    assert isinstance(result, RealESRGANer)


@requires_cuda
@pytest.mark.parametrize("filename,scale", MODELS)
def test_load_model_scale_matches(filename: str, scale: int) -> None:
    result = _load_model(WEIGHTS_DIR / filename, scale=scale)

    assert result.scale == scale


@requires_cuda
@pytest.mark.parametrize("filename,scale", MODELS)
def test_load_model_is_half_precision(filename: str, scale: int) -> None:
    result = _load_model(WEIGHTS_DIR / filename, scale=scale)

    param = next(result.model.parameters())
    assert param.dtype == torch.float16


@requires_cuda
@pytest.mark.parametrize("filename,scale", MODELS)
def test_load_model_tile_disabled(filename: str, scale: int) -> None:
    result = _load_model(WEIGHTS_DIR / filename, scale=scale)

    assert result.tile_size == 0
