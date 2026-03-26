from ..core.logging import logger
from .video import split_into_chunks
from ..nats.messages import VideoChunkMessage, SceneSplitMessage
from ..nats.publisher import scene_video_chunks
from scenedetect import VideoOpenFailure
from nats.js.client import JetStreamContext


async def process_job(metadata: SceneSplitMessage, js: JetStreamContext) -> None:
    """
    Entire processing pipeline, takes in the msg from NATS subcriber
    splits the video into chunks, and calls the publisher to push msgs with chunk data

    Args:
        metadata: the nats message containing the job_id and storage_path
        js: jetstream context for sending nats msgs onto jetstream

    Raises:
        VideoOpenFailure: if the video exists but scenedetect is unable to open it for some
        reason, logs and raises
        OSError: if the video isnt found like not existing on the filepath, logs and raises
    """
    try:
        chunk_paths = split_into_chunks(metadata.storage_path, output_dir="../temp")
    except VideoOpenFailure as e:
        logger.error("could not open video", job_id=metadata.job_id, err=str(e))
        raise
    except OSError as e:
        logger.error(
            "ffmpeg error while splitting video", job_id=metadata.job_id, err=str(e)
        )
        raise

    chunk_messages = [
        VideoChunkMessage(job_id=metadata.job_id, chunk_index=i, storage_path=path)
        for i, path in enumerate(chunk_paths)
    ]

    await scene_video_chunks(js, chunk_messages)
