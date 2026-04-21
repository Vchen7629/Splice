from shared_core.logging import get_logger
from shared_core.settings import settings
import requests


def check_storage_health(service_name: str) -> None:
    """
    Check if seaweedfs filer is reachable

    Args:
        service_name: the service name to log with

    Raises:
        requests.ConnectionError: if seaweedfs is unreachable
        requests.HTTPError: if seaweedfs filer returns a 5xx error
    """
    logger = get_logger(service_name)

    try:
        response = requests.get(settings.BASE_STORAGE_URL + "/")
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
