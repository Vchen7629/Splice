from pathlib import Path
from typing import Any, Generator
from unittest.mock import MagicMock, call, patch

import pytest

from processing.upscale_ml_model import Resolution, _load_model, select_model


@pytest.fixture
def mock_deps() -> Generator[dict[str, Any], Any, None]:
    mock_net = MagicMock()
    mock_net.cuda.return_value = mock_net
    mock_net.half.return_value = mock_net
    mock_net.eval.return_value = mock_net

    with patch("processing.upscale_ml_model.ModelLoader") as mock_loader:
        mock_loader.return_value.load_from_file.return_value.model = mock_net
        yield {"loader": mock_loader, "net": mock_net}


@pytest.mark.parametrize(
    "s,expected",
    [
        ("280p", Resolution.R_280P),
        ("360p", Resolution.R_360P),
        ("480p", Resolution.R_480P),
        ("720p", Resolution.R_720P),
        ("960p", Resolution.R_960P),
        ("1080p", Resolution.R_1080P),
        ("1440p", Resolution.R_1440P),
    ],
)
def test_from_string_parses_valid_resolution(s: str, expected: Resolution) -> None:
    assert Resolution.from_string(s) == expected


@pytest.mark.parametrize(
    "s,expected",
    [
        ("144p", Resolution.R_280P),  # nearest to an off-scale detected height
        (
            "1040p",
            Resolution.R_1080P,
        ),  # e.g. a screen-capture with a non-standard height
        ("4k", Resolution.R_2160P),
        ("4K", Resolution.R_2160P),
    ],
)
def test_from_string_snaps_to_nearest_resolution(s: str, expected: Resolution) -> None:
    assert Resolution.from_string(s) == expected


@pytest.mark.parametrize("s", ["", "abcp"])
def test_from_string_raises_for_unparseable_resolution(s: str) -> None:
    with pytest.raises(ValueError):
        Resolution.from_string(s)


@pytest.mark.parametrize(
    "source,target",
    [
        ("1080p", "480p"),
        ("720p", "360p"),
        ("480p", "480p"),  # same resolution
    ],
)
def test_select_model_returns_none_when_ratio_lte_1(source: str, target: str) -> None:
    assert select_model(source, target) is None


@pytest.mark.parametrize(
    "source,target",
    [
        ("480p", "960p"),  # ratio = 2
        ("480p", "1080p"),  # ratio = 2.25
        ("720p", "1440p"),  # ratio = 2
    ],
)
def test_select_model_returns_x2_model_for_ratio_lt_4(source: str, target: str) -> None:
    result = select_model(source, target)

    assert result is not None
    model_path, scale = result
    assert scale == 2
    assert model_path.name == "RealESRGANv2-animevideo-xsx2.pth"


@pytest.mark.parametrize(
    "source,target",
    [
        ("360p", "1440p"),  # ratio = 4
        ("280p", "1440p"),  # ratio > 4
    ],
)
def test_select_model_returns_x4_model_for_ratio_gte_4(
    source: str, target: str
) -> None:
    result = select_model(source, target)

    assert result is not None
    model_path, scale = result
    assert scale == 4
    assert model_path.name == "realesr-animevideov3.pth"


@pytest.mark.parametrize(
    "source,target",
    [
        ("480p", "960p"),  # x2
        ("360p", "1440p"),  # x4
    ],
)
def test_select_model_returned_path_exists(source: str, target: str) -> None:
    result = select_model(source, target)

    assert result is not None
    model_path, _ = result
    assert model_path.exists()


@pytest.mark.parametrize("scale", [1, 3, 8])
def test_load_model_invalid_scale_raises(mock_deps: dict[str, Any], scale: int) -> None:
    with pytest.raises(ValueError, match="scale must be either 2 or 4"):
        _load_model(Path("model.pth"), scale=scale)


@pytest.mark.parametrize("scale", [2, 4])
def test_load_model_valid_scale_does_not_raise(
    mock_deps: dict[str, Any], scale: int
) -> None:
    _load_model(Path("model.pth"), scale=scale)


def test_load_model_loads_weights_from_given_path(mock_deps: dict[str, Any]) -> None:
    model_path = Path("/some/path/model.pth")
    _load_model(model_path, scale=2)

    mock_deps["loader"].return_value.load_from_file.assert_called_once_with(model_path)


def test_load_model_moves_to_cuda_fp16_eval_then_compiles(
    mock_deps: dict[str, Any],
) -> None:
    _load_model(Path("model.pth"), scale=2)

    assert mock_deps["net"].method_calls == [
        call.cuda(),
        call.half(),
        call.eval(),
        call.compile(mode="reduce-overhead"),
    ]


def test_load_model_returns_compiled_model(mock_deps: dict[str, Any]) -> None:
    result = _load_model(Path("model.pth"), scale=2)

    assert result is mock_deps["net"]
