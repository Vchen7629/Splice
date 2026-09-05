from shared_core import get_logger
from shared_storage import fetch_video, upload_video
from shared_handler import VideoChunkMessage, ProcessJobMessage, JobCancelledError
from shared_util import cleanup_temp_dir
from ..core.settings import settings
from .video import split_into_chunks
from scenedetect import VideoOpenFailure
from threading import Event
from typing import Callable, Optional
import os
import asyncio

logger = get_logger(settings.SERVICE_NAME)


async def process_job(
    cancel_event: Event,
    metadata: ProcessJobMessage,
    on_progress: Optional[Callable[[int], None]] = None,
) -> list[VideoChunkMessage]:
    """
    takes in the msg from NATS subcriber, fetches the video from SeaweedFS, splits
    the video into chunks, uploads the chunks back to seaweedfs, and returns
    a list of chunk_messages

    Args:
        cancel_event: event that is triggered when there is a new cancel.{job_id} nats msg to stop
        split_into_chunks from further processing
        metadata: the nats message containing the job_id, storage_url, and target_resolution
        on_progress: optional callback invoked with 0-100 as split_into_chunks proceeds

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

        try:
            chunk_paths = await asyncio.to_thread(
                split_into_chunks,
                logger,
                cancel_event,
                local_video_path,
                chunks_dir,
                on_progress,
            )
        except JobCancelledError:
            raise
        except VideoOpenFailure as e:
            logger.error("could not open video", job_id=metadata.job_id, err=str(e))
            raise
        except OSError as e:
            logger.error(
                "ffmpeg error while splitting video", job_id=metadata.job_id, err=str(e)
            )
            raise

        results = await asyncio.gather(
            *[
                asyncio.to_thread(
                    upload_video,
                    f"{settings.BASE_STORAGE_URL}/{metadata.job_id}/{os.path.basename(path)}",
                    metadata.job_id,
                    path,
                    settings.SERVICE_NAME,
                )
                for path in chunk_paths
            ],
            return_exceptions=True,
        )

        storage_urls: list[str] = []
        for result in results:
            if isinstance(result, BaseException):
                raise result
            storage_urls.append(result)

    finally:
        await cleanup_temp_dir(temp_dir, metadata.job_id, logger)

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
