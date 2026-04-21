from shared_core.logging import get_logger
from shared_core.settings import settings
from nats.js.client import JetStreamContext
from nats.aio.client import Client as NATSClient
import nats.js.errors as js_errors

async def check_js_stream_exists(js: JetStreamContext, subject_name: str, service_name: str) -> None:
    """
    Check if a js stream exists using the subject name. Used before trying to
    connect to the stream in order to fail early

    Args:
        js: the jetstream context connection
        subject_name: the stream subject name we are checking
        service_name: the name of the service for logging

    Raises:
        RuntimeError if the jetstream stream doesnt exist
    """
    try:
        await js.find_stream_name_by_subject(subject_name)
    except js_errors.NotFoundError:
        raise RuntimeError(f"No stream found for `{subject_name}`")


async def nats_connect() -> tuple[NATSClient, JetStreamContext]:
    """nats connection and jetstream context required for pub/sub"""
    nats_url = settings.NATS_URL  # the nats server url

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


async def _on_reconnect() -> None:
    """callback function for logging reconnection"""
    logger.debug("reconnected to nats")


async def _on_disconnect() -> None:
    """callback function for logging disconnect"""
    logger.warning("disconnected from nats")


async def _on_error(err: Exception) -> None:
    """callback function for logging error connecting to nats"""
    logger.error("error connecting to nats", err=str(err))
