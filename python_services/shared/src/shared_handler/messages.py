import re
from typing import Literal, Protocol

from pydantic import BaseModel, field_validator, model_validator

DENOISE_QUALITY = Literal["light", "medium"]
OPERATIONS = Literal["transcode", "denoise", "upscale"]

# job_id matching uuid.New().string() in go services
_JOB_ID_RE = re.compile(r"^[A-Za-z0-9_-]+$")


def _validate_job_id(job_id: str) -> str:
    """guard against non uuid.New().string() job_ids for security"""
    if not _JOB_ID_RE.fullmatch(job_id):
        raise ValueError(f"invalid job_id: {job_id!r}")
    return job_id


class Msg(Protocol):
    operation: OPERATIONS
    denoise_quality: DENOISE_QUALITY | None
    target_resolution: str | None


def _check_operation_fields(msg: Msg) -> Msg:
    if msg.operation == "denoise" and msg.denoise_quality is None:
        raise ValueError("denoise_quality required for denoise")
    if msg.operation in ("transcode", "upscale") and msg.target_resolution is None:
        raise ValueError("target_resolution required for transcode/upscale")
    return msg


class VideoChunkMessage(BaseModel):
    job_id: str
    operation: OPERATIONS
    chunk_index: int
    total_chunks: int
    storage_url: str
    target_resolution: str | None = None
    denoise_quality: DENOISE_QUALITY | None = None

    _check = model_validator(mode="after")(_check_operation_fields)


class ProcessJobMessage(BaseModel):
    job_id: str
    operation: OPERATIONS
    storage_url: str
    source_resolution: str
    target_resolution: str | None = None
    denoise_quality: DENOISE_QUALITY | None = None

    _validate_job_id = field_validator("job_id")(_validate_job_id)
    _check = model_validator(mode="after")(_check_operation_fields)


class UpscaleCompleteMsg(BaseModel):
    job_id: str


class ProgressMessage(BaseModel):
    job_id: str
    stage: str
    progress: int
