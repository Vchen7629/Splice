from pathlib import Path
from pydantic_settings import BaseSettings

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"


class Settings(BaseSettings):
    # general config
    HTTP_PORT: int = 9098
    SERVICE_NAME: str = "scene-detector"

    # Nats config
    SUB_QUEUE_NAME: str = "scene-detector-workers"
    SUB_SUBJECT: str = "jobs.video.scene-split"
    PUB_SUBJECT: str = "jobs.video.chunks"

    BASE_STORAGE_URL: str = "http://localhost:8888"


settings = Settings()
