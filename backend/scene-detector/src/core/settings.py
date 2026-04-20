from pathlib import Path
from pydantic_settings import BaseSettings

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"


class Settings(BaseSettings):
    # general config
    HTTP_PORT: int = 9098

    # Nats config
    NATS_SUB_QUEUE_NAME: str = "scene-detector-workers"
    SCENE_SPLIT_SUBJECT: str = (
        "jobs.video.scene-split"  # topic containing Job ID + storage path in MinIO
    )
    VIDEO_CHUNKS_SUBJECT: str = "jobs.video.chunks"

    MAX_DELIVER_ATTEMPTS: int = 3
    ACK_WAIT_S: int = 30


settings = Settings()
