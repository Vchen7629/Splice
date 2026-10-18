import asyncio
import os
from threading import Event
from typing import Callable, Optional

from shared_core import get_logger, sharedsettings
from shared_handler import JobCancelledError, ProcessJobMessage, VideoChunkMessage
from shared_storage import fetch_video, upload_video
from shared_util import cleanup_temp

from ..core.settings import settings
from .video import split_into_chunks

logger = get_logger(settings.SERVICE_NAME)
TEMP_ROOT = sharedsettings.TEMP_ROOT


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
    try:
        local_video_path = await asyncio.to_thread(
            fetch_video, metadata.storage_url, settings.SERVICE_NAME
        )

        chunk_paths = await asyncio.to_thread(
            split_into_chunks,
            cancel_event,
            local_video_path,
            f"{TEMP_ROOT}/input/{metadata.job_id}/chunks",  # chunks dir
            on_progress,
        )

        if cancel_event.is_set():
            raise JobCancelledError(
                "job cancelled after scene-split, before chunk upload"
            )

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

        if cancel_event.is_set():
            raise JobCancelledError("job cancelled during chunk upload")

    finally:
        await cleanup_temp(
            f"{TEMP_ROOT}/input/{metadata.job_id}", metadata.job_id, logger
        )

    return [
        VideoChunkMessage(
            job_id=metadata.job_id,
            chunk_index=i,
            total_chunks=len(storage_urls),
            storage_url=url,
            target_resolution=metadata.target_resolution,
            operation=metadata.operation,
        )
        for i, url in enumerate(storage_urls)
    ]
