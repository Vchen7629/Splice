class JobCancelledError(Exception):
    """Raised when a threading.Event set by a cancel.{job_id} broadcast
    interrupts work already in progress"""

    pass


class VideoNotFoundErr(Exception):
    pass
