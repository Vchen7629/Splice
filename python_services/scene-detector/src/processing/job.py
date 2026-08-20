from shared_core.logging import get_logger
from shared_storage.queries import fetch_video
from shared_storage.queries import upload_video
from shared_handler.messages import VideoChunkMessage
from shared_handler.messages import ProcessJobMessage
from ..core.settings import settings
from .video import split_into_chunks
from scenedetect import VideoOpenFailure
import os
import asyncio
import shutil

logger = get_logger(settings.SERVICE_NAME)


async def _split_into_chunks(
    local_video_path: str, chunks_dir: str, job_id: str
) -> list[str]:
    """split the video into chunks in a thread and raise on errors"""
    try:
        chunk_paths = await asyncio.to_thread(
            split_into_chunks, local_video_path, chunks_dir
        )
    except VideoOpenFailure as e:
        logger.error("could not open video", job_id=job_id, err=str(e))
        raise
    except OSError as e:
        logger.error(
            "ffmpeg error while splitting video", job_id=job_id, err=str(e)
        )
        raise
    

async def process_job(metadata: ProcessJobMessage) -> list[VideoChunkMessage]:
    """
    takes in the msg from NATS subcriber, fetches the video from SeaweedFS, splits
    the video into chunks, uploads the chunks back to seaweedfs, and returns
    a list of chunk_messages

    Args:
        metadata: the nats message containing the job_id, storage_url, and target_resolution

    Raises:
        requests.ConnectionError: if seaweedfs is unreachable during fetch or upload
        requests.HTTPError: If seaweedfs returns an error during fetch or upload
        FileNotFOundError: If a local chunk video file is missing before upload
        VideoOpenFailure: if scenedetect is unable to open the video
        OSError: if the video isnt found like not existing on the filepath, logs and raises

    Returns:
        list of videochunkmessage with SeaweedFS storage URLS
    """
    temp_dir = f"../temp/{metadata.job_id}"
    chunks_dir = f"../temp/{metadata.job_id}/chunks"

    try:
        local_video_path = await asyncio.to_thread(
            fetch_video, metadata.storage_url, settings.SERVICE_NAME
        )

        chunk_paths = _split_into_chunks(local_video_path, chunks_dir, metadata.job_id)
        
        storage_urls = await asyncio.gather(
            *[
                asyncio.to_thread(
                    upload_video,
                    f"{settings.BASE_STORAGE_URL}/{metadata.job_id}/{os.path.basename(path)}",
                    metadata.job_id,
                    path,
                    settings.SERVICE_NAME,
                )
                for path in chunk_paths
            ]
        )

    finally:
        if os.path.exists(temp_dir):
            try:
                await asyncio.to_thread(lambda: shutil.rmtree(temp_dir))
            except OSError as e:
                logger.warning("failed to clean up temp dir", temp_dir=temp_dir, err=str(e))

    return [
        VideoChunkMessage(
            job_id=metadata.job_id,
            chunk_index=i,
            total_chunks=len(storage_urls),
            storage_url=url,
            target_resolution=metadata.target_resolution,
        )
        for i, url in enumerate(storage_urls)
    ]
