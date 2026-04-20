from shared_core.logging import logger
from shared_core.settings import settings
import os
import requests

TEMP_DIR: str = "../temp"


def fetch_video(storage_url: str) -> str:
    """
    Fetch unprocessed video from seaweedfs storage for processing
    and save it locally for processing

    Args:
        storage_url: full SeaweedFS URL to the video, from NATS

    Raises:
        requests.ConnectionError if SeaweedFS is unreachable
        request.HTTPError: if SeaweedFS returns 404 or 5xx

    Returns:
        dest_path string on success
    """
    try:
        response = requests.get(storage_url)
        response.raise_for_status()
    except requests.ConnectionError as e:
        logger.error(
            "could not connect to seaweedfs", storage_url=storage_url, err=str(e)
        )
        raise
    except requests.HTTPError as e:
        logger.error(
            "seaweedfs returned error fetching video",
            storage_url=storage_url,
            status_code=e.response.status_code,
            err=str(e),
        )
        raise

    parts = storage_url.rstrip("/").split("/")
    dest_path: str = f"{TEMP_DIR}/{parts[-2]}/{parts[-1]}"
    os.makedirs(os.path.dirname(dest_path), exist_ok=True)
    with open(dest_path, "wb") as f:
        f.write(response.content)

    return dest_path


def upload_video(job_id: str, video_path: str) -> str:
    """
    Upload a single video to seaweedfs storage

    Args:
        job_id: job_id for one request from NATS
        video_path: local file path for the video

    Raises:
        FileNotFoundError: if the local chunk file is missing before upload
        requests.ConnectionError: If SeaweedFS is unreachable
        requests.HTTPError: If SeaweedFS returns 4xx/5xx on upload

    Returns:
        SeaweedFS storage URL for the uploaded video
    """
    if not os.path.exists(video_path):
        logger.error(
            "video file not found before upload",
            chunk_path=video_path,
            job_id=job_id,
        )
        raise FileNotFoundError(f"video file not found: {video_path}")

    filename = os.path.basename(video_path)
    url = f"{settings.BASE_STORAGE_URL}/{job_id}/{filename}"

    try:
        with open(video_path, "rb") as f:
            response = requests.put(
                url, data=f, headers={"Content-Type": "application/octet-stream"}
            )
        response.raise_for_status()
    except requests.ConnectionError as e:
        logger.error(
            "could not connect to seaweedfs", url=url, job_id=job_id, err=str(e)
        )
        raise
    except requests.HTTPError as e:
        logger.error(
            "seaweedfs returned error uploading video",
            url=url,
            job_id=job_id,
            status_code=e.response.status_code,
            err=str(e),
        )
        raise

    logger.debug("uploaded video to seaweedfs", job_id=job_id, url=url)
    return url
