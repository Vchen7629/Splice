from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
import requests

from shared_storage import TEMP_UNPROCESSED_STORAGE_TTL, upload_video

fake_storage_url = "idk/idk2/chunk-001.mp4"


def test_raises_file_not_found(tmp_path: Path) -> None:
    """Raises FileNotFoundError when the chunk path does not exist on disk"""
    with pytest.raises(FileNotFoundError):
        upload_video(
            fake_storage_url,
            "job-1",
            str(tmp_path / "missing.mp4"),
            service_name="scene-detector",
            storage_ttl=TEMP_UNPROCESSED_STORAGE_TTL,
        )


@pytest.mark.parametrize("status_code", [400, 404, 500, 503])
def test_raises_on_http_error(single_video_chunk: str, status_code: int) -> None:
    """Raises HTTPError when SeaweedFS returns 4xx/5xx on upload"""
    mock_response = MagicMock()
    mock_response.status_code = status_code
    mock_response.raise_for_status.side_effect = requests.HTTPError(
        response=mock_response
    )

    with (
        patch("shared_storage.queries.requests.put", return_value=mock_response),
        pytest.raises(requests.HTTPError),
    ):
        upload_video(
            fake_storage_url,
            "job-1",
            single_video_chunk,
            service_name="scene-detector",
            storage_ttl=TEMP_UNPROCESSED_STORAGE_TTL,
        )


def test_raises_on_connection_error(single_video_chunk: str) -> None:
    """Raises ConnectionError when SeaweedFS is unreachable during upload"""
    with (
        patch(
            "shared_storage.queries.requests.put", side_effect=requests.ConnectionError
        ),
        pytest.raises(requests.ConnectionError),
    ):
        upload_video(
            fake_storage_url,
            "job-1",
            single_video_chunk,
            service_name="scene-detector",
            storage_ttl=TEMP_UNPROCESSED_STORAGE_TTL,
        )


def test_returns_correct_storage_url(fake_base_url: str, tmp_path: Path) -> None:
    """Returns SeaweedFS URL matching {base}/{job_id}/{filename}"""
    job_id = "job-abc"
    chunk = tmp_path / "chunk-001.mp4"
    chunk.write_bytes(b"data")

    mock_response = MagicMock()
    mock_response.raise_for_status.return_value = None

    storage_url = f"{fake_base_url}/{job_id}/chunk-001.mp4"

    with patch("shared_storage.queries.requests.put", return_value=mock_response):
        url = upload_video(
            storage_url,
            job_id,
            str(chunk),
            service_name="scene-detector",
            storage_ttl=TEMP_UNPROCESSED_STORAGE_TTL,
        )

    assert url == storage_url


def test_upload_sends_temp_ttl_param(single_video_chunk: str) -> None:
    """Uploads pass the 1d temporary-artifact TTL as a query param"""
    mock_response = MagicMock()
    mock_response.raise_for_status.return_value = None

    with patch(
        "shared_storage.queries.requests.put", return_value=mock_response
    ) as mock_put:
        upload_video(
            fake_storage_url,
            "job-1",
            single_video_chunk,
            service_name="scene-detector",
            storage_ttl=TEMP_UNPROCESSED_STORAGE_TTL,
        )

    assert mock_put.call_args.kwargs["params"] == {"ttl": "1d"}
