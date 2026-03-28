# Splice
Distributed video transcoding system. Submit a video via CLI, Api, or web ui via local file, s3 bucket, or google cloud service. The system auto detects scene boundaries, splits the video into chunks, and utilizes ffmpeg and worker nodes to transcode each chunk in parallel to massively speed up video transcoding
