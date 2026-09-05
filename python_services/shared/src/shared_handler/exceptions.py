class JobCancelledError(Exception):
    """Raised when a threading.Event set by a cancel.{job_id} broadcast
    interrupts work already in progress (scene-detector's detect/scene
    loops, video-upscaling's frame loop)."""
