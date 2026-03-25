from nats.js.client import JetStreamContext
from nats.errors import TimeoutError
from nats.js.errors import APIError
from ..core.logging import logger
from ..core.settings import settings
from .messages import VideoChunkMessage


async def scene_video_chunks(
    js: JetStreamContext, msgs: list[VideoChunkMessage]
) -> None:
    """
    Publishes video split by scene ready message to nats jetstream

    Args:
        js: the jetstream context with connection info for publishing
        msgs: the actual data we are publishing to the broker

    Raises:
        TimeoutError: when publishing times out, logs and raises
        APIError: when an jetstream api error is recieved when trying
        to publish, logs and raises
    """
    for msg in msgs:
        try:
            await js.publish(
                subject=settings.VIDEO_CHUNKS_SUBJECT,
                payload=msg.model_dump_json().encode(),
            )
            logger.debug("pub msg to nats jetstream successfully")
        except TimeoutError as e:
            logger.error(
                "timed out publishing chunk msg",
                job_id=msg.job_id,
                chunk_idex=msg.chunk_index,
                err=str(e),
            )
            raise
        except APIError as e:
            logger.error(
                "jetstream error publishing chunk message",
                job_id=msg.job_id,
                chunk_idex=msg.chunk_index,
                err=str(e),
            )
            raise
