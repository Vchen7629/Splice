# Splice

<div align="center">

[![CI](https://github.com/Vchen7629/Splice/actions/workflows/ci.yaml/badge.svg)](https://github.com/Vchen7629/Splice/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/Vchen7629/Splice/graph/badge.svg?token=XT7E5YRZEX)](https://codecov.io/gh/Vchen7629/Splice)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8)](https://go.dev/)
[![Python](https://img.shields.io/badge/Python-3.13-3776AB)](https://www.python.org/)

</div>

Distributed video transcoding system. Submit a video via CLI, Api, or web ui via local file, s3 bucket, or google cloud service. The system auto detects scene boundaries, splits the video into chunks, and utilizes ffmpeg and worker nodes to transcode each chunk in parallel to massively speed up video transcoding
