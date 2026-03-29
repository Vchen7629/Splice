from ..core.logging import logger
from .video import split_into_chunks
from ..nats.messages import SceneSplitMessage
from ..nats.messages import VideoChunkMessage
from scenedetect import VideoOpenFailure
import asyncio


async def process_job(metadata: SceneSplitMessage) -> list[VideoChunkMessage]:
    """
    takes in the msg from NATS subcriber splits the video into chunks, and returns
    a list of chunk_messages

    Args:
        metadata: the nats message containing the job_id and storage_path

    Raises:
        VideoOpenFailure: if the video exists but scenedetect is unable to open it for some
        reason, logs and raises
        OSError: if the video isnt found like not existing on the filepath, logs and raises

    Returns:
        list of videochunkmessage
    """
    try:
        chunk_paths = await asyncio.to_thread(
            split_into_chunks, metadata.storage_path, f"../temp/{metadata.job_id}"
        )
    except VideoOpenFailure as e:
        logger.error("could not open video", job_id=metadata.job_id, err=str(e))
        raise
    except OSError as e:
        logger.error(
            "ffmpeg error while splitting video", job_id=metadata.job_id, err=str(e)
        )
        raise

    return [
        VideoChunkMessage(
            job_id=metadata.job_id,
            chunk_index=i,
            total_chunks=len(chunk_paths),
            storage_path=path,
            target_resolution=metadata.target_resolution,
        )
        for i, path in enumerate(chunk_paths)
    ]
