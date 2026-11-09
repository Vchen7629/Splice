from nats.js import JetStreamContext
from nats.js.errors import APIError

from core import get_logger, settings

from .messages import UpscaleCompleteMsg


async def publisher(js: JetStreamContext, msg: UpscaleCompleteMsg) -> None:
    """
    Publishes message to nats jetstream

    Args:
        js: the jetstream context with connection info for publishing
        msg: the actual data we are publishing to the broker

    Raises:
        TimeoutError: when publishing times out, logs and raises
        APIError: when an jetstream api error is recieved when trying
        to publish, logs and raises
    """
    logger = get_logger(settings.SERVICE_NAME)

    try:
        await js.publish(
            subject=settings.PUB_SUBJECT, payload=msg.model_dump_json().encode()
        )
        logger.debug("pub msg to nats jetstream successfully")
    except TimeoutError as e:
        logger.error("timed out publishing msg", job_id=msg.job_id, err=str(e))
        raise
    except APIError as e:
        logger.error("jetstream error publishing msg", job_id=msg.job_id, err=str(e))
        raise
