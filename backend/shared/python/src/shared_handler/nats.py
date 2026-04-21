from typing import Awaitable
from typing import Callable
from nats.aio.msg import Msg
from nats.js.kv import KeyValue
from nats.js.api import ConsumerConfig
from nats.errors import TimeoutError
from nats.js.errors import APIError
from nats.js.client import JetStreamContext
from shared_core.logging import logger
from shared_core.settings import settings
from shared_handler.messages import UpscaleCompleteMsg
from .messages import VideoChunkMessage


async def consumer(
    js: JetStreamContext,
    msg_processed_kv: KeyValue,
    job_status_kv: KeyValue,
    sub_subject: str,
    durable_name: str,
    queue_name: str,
    process_msg: Callable[[JetStreamContext, KeyValue, KeyValue, Msg], Awaitable[None]],
) -> None:
    """Nats jetstream consumer that subscribes to subject to process videos"""
    sub = await js.subscribe(
        subject=sub_subject,
        durable=durable_name,
        queue=queue_name,
        config=ConsumerConfig(
            max_deliver=settings.MAX_DELIVER_ATTEMPTS, ack_wait=settings.ACK_WAIT_S
        ),
    )

    async for msg in sub.messages:
        await process_msg(js, msg_processed_kv, job_status_kv, msg)


async def publisher(js: JetStreamContext, msg: VideoChunkMessage | UpscaleCompleteMsg, subject: str) -> None:
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
