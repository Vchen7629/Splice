from ..core.logging import logger
from .video import split_into_chunks
from ..nats.messages import VideoChunkMessage, SceneSplitMessage
from ..nats.publisher import scene_video_chunks
from scenedetect import VideoOpenFailure
from scenedetect.platform import CommandTooLong
from nats.js.client import JetStreamContext


async def process_job(metadata: SceneSplitMessage, js: JetStreamContext) -> None:
    """Entire processing pipeline"""
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
    except CommandTooLong as e:
        logger.error("too many scenes to process", job_id=metadata.job_id, err=str(e))
        raise

    chunk_messages = [
        VideoChunkMessage(job_id=metadata.job_id, chunk_index=i, storage_path=path)
        for i, path in enumerate(chunk_paths)
    ]

    await scene_video_chunks(js, chunk_messages)


if __name__ == "__main__":
    import asyncio
    import os

    storage_path = os.path.join(os.path.dirname(__file__), "ForBiggerBlazes.mp4")
    asyncio.run(
        process_job(SceneSplitMessage(job_id="2", storage_path=storage_path), js=None)
    )
