from ..core.logging import logger
from ..core.settings import settings
from nats import NATS
from nats.js.client import JetStreamContext
import nats


async def nats_connect() -> tuple[NATS, JetStreamContext]:
    """nats connection and jetstream context required for pub/sub"""
    nats_url = settings.NATS_URL  # the nats server url

    nats_client = await nats.connect(
        nats_url,
        max_reconnect_attempts=settings.MAX_RECONNECT_ATTEMPT,
        reconnect_time_wait=settings.RECONNECT_TIME_WAIT_S,
        reconnected_cb=_on_reconnect,
        disconnected_cb=_on_disconnect,
        error_cb=_on_error,
    )

    jetstream_client: JetStreamContext = nats_client.jetstream()

    return nats_client, jetstream_client


async def _on_reconnect() -> None:
    """callback function for logging reconnection"""
    logger.debug("reconnected to nats")


async def _on_disconnect() -> None:
    """callback function for logging disconnect"""
    logger.warning("disconnected from nats")


async def _on_error(err: Exception) -> None:
    """callback function for logging error connecting to nats"""
    logger.error("error connecting to nats", err=str(err))
