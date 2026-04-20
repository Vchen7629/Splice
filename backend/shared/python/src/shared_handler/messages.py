from pydantic import BaseModel


class VideoChunkMessage(BaseModel):
    job_id: str
    chunk_index: int
    total_chunks: int
    storage_url: str
    target_resolution: str
