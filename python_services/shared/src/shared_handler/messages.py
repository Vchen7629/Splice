from pydantic import BaseModel, field_validator
import re


class VideoChunkMessage(BaseModel):
    job_id: str
    chunk_index: int
    total_chunks: int
    storage_url: str
    target_resolution: str


# job_id matching uuid.New().string() in go services
_JOB_ID_RE = re.compile(r"^[A-Za-z0-9_-]+$")


def _validate_job_id(job_id: str) -> str:
    """guard against non uuid.New().string() job_ids for security"""
    if not _JOB_ID_RE.fullmatch(job_id):
        raise ValueError(f"invalid job_id: {job_id!r}")
    return job_id


class ProcessJobMessage(BaseModel):
    job_id: str
    storage_url: str
    source_resolution: str
    target_resolution: str

    _validate_job_id = field_validator("job_id")(_validate_job_id)


class UpscaleCompleteMsg(BaseModel):
    job_id: str


class ProgressMessage(BaseModel):
    job_id: str
    stage: str
    progress: int
