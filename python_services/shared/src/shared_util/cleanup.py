import asyncio
from os import path, remove
from shutil import rmtree

from structlog.stdlib import BoundLogger


async def cleanup_temp(
    temp_path: str,
    job_id: str,
    logger: BoundLogger,
    retries: int = 3,
    delay_seconds: float = 1.0,
) -> None:
    """remove the job's temp file or dir, retrying a few times"""
    for attempt in range(1, retries + 1):
        try:
            await asyncio.to_thread(_remove, temp_path)
            return
        except FileNotFoundError:
            return
        except OSError as e:
            if attempt == retries:
                logger.error(
                    "failed to clean up temp path after retries",
                    temp_path=temp_path,
                    job_id=job_id,
                    attempts=attempt,
                    err=str(e),
                )
                return
            await asyncio.sleep(delay_seconds)


def _remove(target: str) -> None:
    """check if the target is a file or a folder and use the correct remove method"""
    if path.isdir(target) and not path.islink(target):
        rmtree(target)
    else:
        remove(target)
