from .settings import settings
import sys
import logging
import structlog


def configure_logging() -> None:
    """Initialize the structured logger"""
    level = logging.DEBUG if settings.LOG_LEVEL == "DEBUG" else logging.INFO
    logging.basicConfig(stream=sys.stdout, level=level)

    processors: list[structlog.types.Processor] = [
        structlog.stdlib.add_log_level,
        structlog.processors.TimeStamper(fmt="iso"),
    ]

    if settings.log_format == "json":
        processors.append(structlog.processors.JSONRenderer())
    else:
        processors.append(structlog.dev.ConsoleRenderer())

    structlog.configure(processors=processors)


logger: structlog.stdlib.BoundLogger = structlog.get_logger().bind(
    service="scene-detector"
)

configure_logging()
