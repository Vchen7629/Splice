from nats.aio.client import Client as NATSClient
from nats.js import JetStreamContext
from nats.js.errors import NotFoundError
from structlog.stdlib import BoundLogger

from core import settings


async def nats_connect(logger: BoundLogger) -> tuple[NATSClient, JetStreamContext]:
    """nats connection and jetstream context required for pub/sub"""
    nats_url = settings.NATS_URL

    async def _on_reconnect() -> None:
        logger.debug("reconnected to nats")

    async def _on_disconnect() -> None:
        logger.warning("disconnected from nats")

    async def _on_error(err: Exception) -> None:
        logger.error("error connecting to nats", err=str(err))

    nats_client = NATSClient()
    await nats_client.connect(
        nats_url,
        max_reconnect_attempts=settings.MAX_RECONNECT_ATTEMPT,
        reconnect_time_wait=settings.RECONNECT_TIME_WAIT_S,
        reconnected_cb=_on_reconnect,
        disconnected_cb=_on_disconnect,
        error_cb=_on_error,
    )

    jetstream_client: JetStreamContext = nats_client.jetstream()

    return nats_client, jetstream_client


async def check_js_stream_exists(js: JetStreamContext, subject_name: str) -> None:
    """
    Check if a js stream exists using the subject name. Used before trying to
    connect to the stream in order to fail early

    Args:
        js: the jetstream context connection
        subject_name: the stream subject name we are checking

    Raises:
        RuntimeError if the jetstream stream doesnt exist
    """
    try:
        await js.find_stream_name_by_subject(subject_name)
    except NotFoundError:
        raise RuntimeError(f"No stream found for `{subject_name}`")
