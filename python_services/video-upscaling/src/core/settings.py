from pathlib import Path

from pydantic_settings import SettingsConfigDict
from shared_core.settings import SharedSettings

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"


class Settings(SharedSettings):
    model_config = SettingsConfigDict(env_file=ENV_FILE)

    # general config
    HTTP_PORT: int = 9101
    BATCH_SIZE: int = 4
    SERVICE_NAME: str = "video-upscaling"

    # Nats config
    SUB_SUBJECT: str = "jobs.video.upscale"
    SUB_QUEUE_NAME: str = "video-upscaling-workers"
    PUB_SUBJECT: str = "jobs.complete"


settings = Settings()
