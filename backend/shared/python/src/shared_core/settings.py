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
    MAX_RECONNECT_ATTEMPT: int = 5
    RECONNECT_TIME_WAIT_S: int = 2
    KV_BUCKET_TTL_S: int = 3 * 60 * 60  # 3 hour TTL
    MAX_DELIVER_ATTEMPTS: int = 3
    ACK_WAIT_S: int = 30

    BASE_STORAGE_URL: str = "http://localhost:8888"


settings = Settings()
