from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=ENV_FILE)

    # general config
    LOG_LEVEL: str = "DEBUG"
    LOG_FORMAT: str = "json"
    TEMP_ROOT: Path = Path("/tmp/splice")
    HTTP_PORT: int = 9101
    BATCH_SIZE: int = 4
    SERVICE_NAME: str = "video-upscaling"

    # ffmpeg/ffprobe timeout config
    FFPROBE_TIMEOUT_S: int = 60
    FFMPEG_TIMEOUT_GRACE_S: int = (
        15  # fixed startup/IO slack shared by all ffmpeg calls
    )
    RECOMBINE_TIMEOUT_FACTOR: float = 0.5  # seconds allowed per second of video
    DOWNSCALE_TIMEOUT_FACTOR: float = 4.0
    UPSCALE_2X_TIMEOUT_S_PER_FRAME: float = 0.5
    UPSCALE_4X_TIMEOUT_S_PER_FRAME: float = (
        2.0  # placeholder until i get real logs for 4x
    )

    # storage config
    BASE_STORAGE_URL: str = "http://localhost:8888"
    STORAGE_CONNECT_TIMEOUT_S: float = 5
    STORAGE_READ_TIMEOUT_S: float = 30

    # Nats config
    NATS_URL: str = "nats://localhost:4222"
    SUB_SUBJECT: str = "jobs.video.upscale"
    SUB_QUEUE_NAME: str = "video-upscaling-workers"
    PUB_SUBJECT: str = "jobs.complete"
    MAX_RECONNECT_ATTEMPT: int = 5
    RECONNECT_TIME_WAIT_S: int = 2
    KV_BUCKET_TTL_S: int = 3 * 60 * 60  # 3 hour TTL
    MAX_DELIVER_ATTEMPTS: int = 3
    ACK_WAIT_S: int = 30


settings = Settings()
