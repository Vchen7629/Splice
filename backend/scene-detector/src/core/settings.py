from pathlib import Path
from pydantic_settings import BaseSettings

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"


class Settings(BaseSettings):
    # general config
    LOG_LEVEL: str = "DEBUG"
    LOG_FORMAT: str = "json"

    # Nats config
    NATS_URL: str = "nats://localhost:4222"
    NATS_SUB_QUEUE_NAME: str = "scene-detetector-workers"
    SCENE_SPLIT_SUBJECT: str = (
        "jobs.video.scene-split"  # topic containing Job ID + storage path in MinIO
    )
    VIDEO_CHUNKS_SUBJECT: str = "jobs.video.chunks"

    MAX_RECONNECT_ATTEMPT: int = 5
    RECONNECT_TIME_WAIT_S: int = 2


settings = Settings()
