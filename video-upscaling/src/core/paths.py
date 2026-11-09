from .settings import settings


def input_dir(job_id: str) -> str:
    return f"{settings.TEMP_ROOT}/input/{job_id}"


def output_dir(job_id: str) -> str:
    return f"{settings.TEMP_ROOT}/output/{job_id}"
