from ..core.logging import logger
from ..storage.queries import fetch_video
from ..storage.queries import upload_video_chunks
from .video import split_into_chunks
from ..nats.messages import SceneSplitMessage
from ..nats.messages import VideoChunkMessage
from scenedetect import VideoOpenFailure
import asyncio
import shutil


async def process_job(metadata: SceneSplitMessage) -> list[VideoChunkMessage]:
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

    local_video_path = await asyncio.to_thread(fetch_video, metadata.storage_url)

    try:
        chunk_paths = await asyncio.to_thread(
            split_into_chunks, local_video_path, chunks_dir
        )
    except VideoOpenFailure as e:
        logger.error("could not open video", job_id=metadata.job_id, err=str(e))
        raise
    except OSError as e:
        logger.error(
            "ffmpeg error while splitting video", job_id=metadata.job_id, err=str(e)
        )
        raise

    chunk_paths = await asyncio.to_thread(
        upload_video_chunks, metadata.job_id, chunk_paths
    )

    try:
        await asyncio.to_thread(lambda: shutil.rmtree(temp_dir))
    except OSError as e:
        logger.warning("failed to clean up temp dir", temp_dir=temp_dir, err=str(e))

    return [
        VideoChunkMessage(
            job_id=metadata.job_id,
            chunk_index=i,
            total_chunks=len(chunk_paths),
            storage_url=path,
            target_resolution=metadata.target_resolution,
        )
        for i, path in enumerate(chunk_paths)
    ]
