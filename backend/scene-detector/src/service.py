from .nats.subscriber import raw_videos
from .nats.connection import nats_connect
from .storage.check_health import check_storage_health
from .core.logging import logger
from .core.settings import settings
import nats.js.errors as js_errors
import asyncio


async def start_service() -> None:
    """Start the python scene-detection service"""
    check_storage_health()

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
        await raw_videos(js)
    finally:
        await nc.drain()


if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(start_service())
