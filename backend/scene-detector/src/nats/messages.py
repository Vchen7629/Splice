from pydantic import BaseModel


# typed class for messages in the nats jetstream
class SceneSplitMessage(BaseModel):
    job_id: str
    storage_path: str


class VideoChunkMessage(BaseModel):
    job_id: str
    chunk_index: int
    storage_path: str
