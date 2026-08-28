from src.utils import Resolution, select_model
import pytest


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
