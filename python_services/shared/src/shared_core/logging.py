from .settings import settings
import sys
import logging
import structlog


def configure_logging() -> None:
    """Initialize structlog, call once at service startup"""
    level = logging.DEBUG if settings.LOG_LEVEL == "DEBUG" else logging.INFO
    logging.basicConfig(stream=sys.stdout, level=level)

    processors: list[structlog.types.Processor] = [
        structlog.stdlib.add_log_level,
        structlog.processors.TimeStamper(fmt="iso"),
    ]

    if settings.LOG_FORMAT == "json":
        processors.append(structlog.processors.JSONRenderer())
    else:
        processors.append(structlog.dev.ConsoleRenderer())

    structlog.configure(processors=processors)


def get_logger(service_name: str) -> structlog.stdlib.BoundLogger:
    return structlog.get_logger().bind(service=service_name)


configure_logging()
