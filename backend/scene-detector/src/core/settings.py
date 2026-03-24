from pathlib import Path
from pydantic_settings import BaseSettings

PROJECT_ROOT = Path(__file__).parent.parent.parent
ENV_FILE = PROJECT_ROOT / ".env"

class Settings(BaseSettings):
    # general config
    LOG_LEVEL: str = "DEBUG"

    # Nats config
    NATS_URL: str = "nats://localhost:4222"
    NATS_QUEUE_NAME: str = "scene-detetector-workers" 
    RAW_VIDEO_SUBJECT: str = "scene-detector" # kafka topic equivalent

    MAX_RECONNECT_ATTEMPT: int = 5
    RECONNECT_TIME_WAIT_S: int = 2

settings = Settings()