from pydantic import BaseModel


class SceneSplitMessage(BaseModel):
    job_id: str
    storage_url: str
    target_resolution: str
