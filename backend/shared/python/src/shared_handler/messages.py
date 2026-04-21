from pydantic import BaseModel


class VideoChunkMessage(BaseModel):
    job_id: str
    chunk_index: int
    total_chunks: int
    storage_url: str
    target_resolution: str

class ProcessJobMessage(BaseModel):
    job_id: str
    storage_url: str
    source_resolution: str
    target_resolution: str

class UpscaleCompleteMsg(BaseModel):
    job_id: str
