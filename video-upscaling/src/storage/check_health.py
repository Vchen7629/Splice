import requests
from structlog.stdlib import BoundLogger

from core import settings


def check_storage_health(logger: BoundLogger) -> None:
    """
    Check if seaweedfs filer is reachable

    Args:
        logger: for logging

    Raises:
        requests.ConnectionError: if seaweedfs is unreachable
        requests.HTTPError: if seaweedfs filer returns a 5xx error
    """
    try:
        response = requests.get(
            settings.BASE_STORAGE_URL + "/",
            timeout=settings.STORAGE_CONNECT_TIMEOUT_S,
        )
        response.raise_for_status()
    except requests.ConnectionError as e:
        logger.error("seaweedfs filer unreachable", err=str(e))
        raise
    except requests.HTTPError as e:
        logger.error(
            "seaweedfs file returned error",
            status_code=e.response.status_code,
            err=str(e),
        )
        raise
