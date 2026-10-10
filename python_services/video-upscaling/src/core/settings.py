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

    # ffmpeg/ffprobe config
    FFPROBE_TIMEOUT_S: int = 60
    FFMPEG_TIMEOUT_GRACE_S: int = (
        15  # fixed startup/IO slack shared by all ffmpeg calls
    )
    RECOMBINE_TIMEOUT_FACTOR: float = 0.5  # seconds allowed per second of video
    DOWNSCALE_TIMEOUT_FACTOR: float = 4.0

    # Nats config
    SUB_SUBJECT: str = "jobs.video.upscale"
    SUB_QUEUE_NAME: str = "video-upscaling-workers"
    PUB_SUBJECT: str = "jobs.complete"


settings = Settings()
