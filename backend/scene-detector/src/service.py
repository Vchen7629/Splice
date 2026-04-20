from nats.js.api import KeyValueConfig
from .handler.subscriber import raw_videos
from .handler.connection import nats_connect
from .handler.http_server import start_health_server
from .storage.check_health import check_storage_health
from .core.logging import logger
from .core.settings import settings
import nats.js.errors as js_errors
import asyncio


async def start_service() -> None:
    """Start the python scene-detection service"""
    check_storage_health()
    health_server = start_health_server(settings.HTTP_PORT)

    nc, js = await nats_connect()

    try:
        await js.find_stream_name_by_subject(settings.SCENE_SPLIT_SUBJECT)
    except js_errors.NotFoundError:
        raise RuntimeError(
            f"No stream found for subscriber `{settings.SCENE_SPLIT_SUBJECT}`"
        )

    try:
        await js.find_stream_name_by_subject(settings.VIDEO_CHUNKS_SUBJECT)
    except js_errors.NotFoundError:
        raise RuntimeError(
            f"No stream found for video chunks subject `{settings.VIDEO_CHUNKS_SUBJECT}`"
        )

    try:
        msg_processed_kv = await js.create_key_value(
            config=KeyValueConfig(
                bucket="scene-split-processed",
                description="key value bucket for scene detector to check if the job_id already processed for idempotency",
                ttl=settings.KV_BUCKET_TTL_S,
            )
        )
    except js_errors.APIError as e:
        raise RuntimeError(f"failed to create scene-split-processed KV bucket: {e}")

    try:
        job_status_kv = await js.key_value("job-status")
    except js_errors.NotFoundError:
        raise RuntimeError(
            "job-status KV bucket not found, check video-status is running"
        )

    try:
        await raw_videos(js, msg_processed_kv, job_status_kv)
    finally:
        health_server.shutdown()
        await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(start_service())
