from pathlib import Path
from realesrgan import RealESRGANer
from src.processing.load_model import load_model
import pytest
import torch

WEIGHTS_DIR = Path(__file__).parent.parent.parent / "src" / "weights"

requires_cuda = pytest.mark.skipif(
    not torch.cuda.is_available(), reason="CUDA not available"
)


@pytest.mark.parametrize(
    "filename,scale",
    [
        ("realesr-animevideov3.pth", 4),
        ("RealESRGANv2-animevideo-xsx2.pth", 2),
    ],
)
@requires_cuda
def test_load_model_returns_realesrganer(filename: str, scale: int) -> None:
    result = load_model(WEIGHTS_DIR / filename, scale=scale)

    assert isinstance(result, RealESRGANer)


@pytest.mark.parametrize(
    "filename,scale",
    [
        ("realesr-animevideov3.pth", 4),
        ("RealESRGANv2-animevideo-xsx2.pth", 2),
    ],
)
@requires_cuda
def test_load_model_scale_matches(filename: str, scale: int) -> None:
    result = load_model(WEIGHTS_DIR / filename, scale=scale)

    assert result.scale == scale


@pytest.mark.parametrize(
    "filename,scale",
    [
        ("realesr-animevideov3.pth", 4),
        ("RealESRGANv2-animevideo-xsx2.pth", 2),
    ],
)
@requires_cuda
def test_load_model_is_half_precision(filename: str, scale: int) -> None:
    result = load_model(WEIGHTS_DIR / filename, scale=scale)

    param = next(result.model.parameters())
    assert param.dtype == torch.float16


@pytest.mark.parametrize(
    "filename,scale",
    [
        ("realesr-animevideov3.pth", 4),
        ("RealESRGANv2-animevideo-xsx2.pth", 2),
    ],
)
@requires_cuda
def test_load_model_tile_disabled(filename: str, scale: int) -> None:
    result = load_model(WEIGHTS_DIR / filename, scale=scale)

    assert result.tile_size == 0
