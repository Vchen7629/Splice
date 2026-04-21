from pathlib import Path
from pydantic_settings import BaseSettings

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"


class Settings(BaseSettings):
    # general config
    HTTP_PORT: int = 9101
    BATCH_SIZE: int = 4
    SERVICE_NAME: str = "video-upscaling"

    # Nats config
    NATS_URL: str = "nats://localhost:4222"
    SUB_SUBJECT: str = "jobs.video.upscale"
    SUB_QUEUE_NAME: str = "video-upscaling-workers"
    PUB_SUBJECT: str = "jobs.complete"
    MAX_DELIVER_ATTEMPTS: int = 3
    ACK_WAIT_S: int = 30


settings = Settings()
