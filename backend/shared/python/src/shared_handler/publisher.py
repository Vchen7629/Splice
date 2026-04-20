from nats.js.client import JetStreamContext
from nats.errors import TimeoutError
from nats.js.errors import APIError
from shared_core.logging import logger
from .messages import VideoChunkMessage


async def publish_jetstream(
    js: JetStreamContext, msg: VideoChunkMessage, subject: str
) -> None:
    """
    Publishes message to nats jetstream

    Args:
        js: the jetstream context with connection info for publishing
        msg: the actual data we are publishing to the broker
        subject: the jetstream subject we want to publish to

    Raises:
        TimeoutError: when publishing times out, logs and raises
        APIError: when an jetstream api error is recieved when trying
        to publish, logs and raises
    """
    try:
        await js.publish(subject=subject, payload=msg.model_dump_json().encode())
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
