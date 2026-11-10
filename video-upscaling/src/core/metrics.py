from prometheus_client import Counter, Histogram

JOB_DURATION = Histogram(
    "video_upscale_job_duration_seconds",
    "e2e time to upscale one video",
    buckets=(30, 60, 120, 300, 600, 1200, 1800, 3600),
)
JOBS_TOTAL = Counter(
    "video_upscale_jobs_total", "Upscale jobs finished", ["status"]
)  # success|cancelled|failure
