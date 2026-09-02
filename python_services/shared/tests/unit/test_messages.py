from shared_handler import ProcessJobMessage
import pytest


def test_process_job_message_accepts_valid_job_id() -> None:
    msg = ProcessJobMessage(
        job_id="3f2504e0-4f89-11d3-9a0c-0305e82c3301",
        storage_url="http://seaweedfs/job-1/input.mp4",
        source_resolution="480p",
        target_resolution="1080p",
    )

    assert msg.job_id == "3f2504e0-4f89-11d3-9a0c-0305e82c3301"


def test_process_job_message_rejects_path_traversal_job_id() -> None:
    with pytest.raises(ValueError):
        ProcessJobMessage(
            job_id="../../etc/passwd",
            storage_url="http://seaweedfs/job-1/input.mp4",
            source_resolution="480p",
            target_resolution="1080p",
        )
