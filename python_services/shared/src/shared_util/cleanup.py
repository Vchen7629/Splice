from os import remove
from shutil import rmtree
from structlog.stdlib import BoundLogger
import asyncio


async def cleanup_temp_dir(
    temp_dir: str,
    job_id: str,
    logger: BoundLogger,
    retries: int = 3,
    delay_seconds: float = 1.0,
) -> None:
    """remove the job's temp dir, retrying a few times"""
    for attempt in range(1, retries + 1):
        try:
            await asyncio.to_thread(lambda: rmtree(temp_dir))
            return
        except FileNotFoundError:
            return
        except OSError as e:
            if attempt == retries:
                logger.error(
                    "failed to clean up temp dir after retries",
                    temp_dir=temp_dir,
                    job_id=job_id,
                    attempts=attempt,
                    err=str(e),
                )
                return
            await asyncio.sleep(delay_seconds)


async def cleanup_temp_file(
    temp_file: str,
    job_id: str,
    logger: BoundLogger,
    retries: int = 3,
    delay_seconds: float = 1.0,
) -> None:
    """remove the job's temp file, retrying a few times"""
    for attempt in range(1, retries + 1):
        try:
            await asyncio.to_thread(lambda: remove(temp_file))
            return
        except FileNotFoundError:
            return
        except OSError as e:
            if attempt == retries:
                logger.error(
                    "failed to clean up temp file after retries",
                    temp_file=temp_file,
                    job_id=job_id,
                    attempts=attempt,
                    err=str(e),
                )
                return
            await asyncio.sleep(delay_seconds)
