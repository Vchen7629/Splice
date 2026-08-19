from typing import Any
from pathlib import Path
from src.processing.load_model import load_model
import pytest


@pytest.mark.parametrize("scale", [1, 3, 8])
def test_invalid_scale_raises(mock_deps: dict[str, Any], scale: int) -> None:
    with pytest.raises(ValueError, match="scale must be either 2 or 4"):
        load_model(Path("model.pth"), scale=scale)


@pytest.mark.parametrize("scale", [2, 4])
def test_valid_scale_does_not_raise(mock_deps: dict[str, Any], scale: int) -> None:
    load_model(Path("model.pth"), scale=scale)


@pytest.mark.parametrize("scale", [2, 4])
def test_srvgg_constructed_with_correct_params(
    mock_deps: dict[str, Any], scale: int
) -> None:
    load_model(Path("model.pth"), scale=scale)

    mock_deps["srvgg"].assert_called_once_with(
        num_in_ch=3,
        num_out_ch=3,
        num_feat=64,
        num_conv=16,
        upscale=scale,
        act_type="prelu",
    )


@pytest.mark.parametrize("scale", [2, 4])
def test_realesrganer_constructed_with_correct_params(
    mock_deps: dict[str, Any], scale: int
) -> None:
    model_path = Path("/some/path/model.pth")
    load_model(model_path, scale=scale)

    mock_deps["realesrgan"].assert_called_once_with(
        scale=scale,
        model_path=str(model_path),
        model=mock_deps["net"],
        tile=0,
        tile_pad=10,
        pre_pad=0,
        device="cuda",
    )


def test_model_converted_to_half_precision(mock_deps: dict[str, Any]) -> None:
    load_model(Path("model.pth"), scale=2)

    mock_deps["net"].half.assert_called_once()


def test_torch_compile_called_with_reduce_overhead(mock_deps: dict[str, Any]) -> None:
    load_model(Path("model.pth"), scale=2)

    mock_deps["compile"].assert_called_once_with(
        mock_deps["net"], mode="reduce-overhead"
    )


def test_returns_upsampler(mock_deps: dict[str, Any]) -> None:
    result = load_model(Path("model.pth"), scale=2)

    assert result is mock_deps["upsampler"]
