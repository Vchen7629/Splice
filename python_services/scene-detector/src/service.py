import asyncio

from shared_core import get_logger
from shared_handler import run_service

from .core.settings import settings
from .processing.nats_msg import process_job_msg

logger = get_logger(settings.SERVICE_NAME)

if __name__ == "__main__":
    logger.debug("starting service")
    asyncio.run(run_service(settings, "scene-split-processed", process_job_msg, None))
