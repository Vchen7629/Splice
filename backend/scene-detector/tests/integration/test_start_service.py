from unittest.mock import patch, AsyncMock
from nats.js import JetStreamContext
from typing import Any
from src.service import start_service
from src.nats.messages import VideoChunkMessage
from src.core.settings import settings
import asyncio
import json
import pytest


@pytest.mark.asyncio
async def test_full_flow_publishes_chunks_downstream(
    js_context: tuple[Any, JetStreamContext],
    nats_video_chunks_subscriber: list[Any],
    monkeypatch: Any,
) -> None:
    """Publishes to upstream topic -> process_job runs -> chunks appear on downstream topic"""
    nc, js = js_context
    monkeypatch.setattr(
        "src.nats.subscriber.settings.SCENE_SPLIT_SUBJECT", settings.SCENE_SPLIT_SUBJECT
    )
    monkeypatch.setattr(
        "src.nats.subscriber.settings.NATS_SUB_QUEUE_NAME", "test-full-flow-worker"
    )
    nc.drain = AsyncMock()

    fake_chunks = [
        VideoChunkMessage(
            job_id="1",
            chunk_index=0,
            total_chunks=2,
            storage_path="/fake/chunk-001.mp4",
            target_resolution="480p",
        ),
        VideoChunkMessage(
            job_id="1",
            chunk_index=1,
            total_chunks=2,
            storage_path="/fake/chunk-002.mp4",
            target_resolution="480p",
        ),
    ]

    async def fake_process_job(_metadata: Any) -> list[VideoChunkMessage]:
        return fake_chunks

    with (
        patch("src.service.nats_connect", return_value=(nc, js)),
        patch("src.nats.subscriber.process_job", side_effect=fake_process_job),
    ):
        task = asyncio.create_task(start_service())
        payload = json.dumps(
            {
                "job_id": "1",
                "storage_path": "/fake/video.mp4",
                "target_resolution": "480p",
            }
        ).encode()
        await nc.publish(settings.SCENE_SPLIT_SUBJECT, payload)
        await asyncio.sleep(0.5)
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert len(nats_video_chunks_subscriber) == 2
    assert nats_video_chunks_subscriber[0] == {
        "job_id": "1",
        "chunk_index": 0,
        "total_chunks": 2,
        "storage_path": "/fake/chunk-001.mp4",
        "target_resolution": "480p",
    }
    assert nats_video_chunks_subscriber[1] == {
        "job_id": "1",
        "chunk_index": 1,
        "total_chunks": 2,
        "storage_path": "/fake/chunk-002.mp4",
        "target_resolution": "480p",
    }


@pytest.mark.asyncio
async def test_raises_runtime_error_when_video_chunks_stream_not_found(
    js_context: tuple[Any, JetStreamContext],
    monkeypatch: Any,
) -> None:
    """Raises RuntimeError when no NATS stream covers the downstream chunks subject"""
    nc, js = js_context
    monkeypatch.setattr(
        "src.service.settings.VIDEO_CHUNKS_SUBJECT", "nonexistent.subject.xyz"
    )
    nc.drain = AsyncMock()

    with patch("src.service.nats_connect", return_value=(nc, js)):
        with pytest.raises(RuntimeError, match="No stream found for video chunks"):
            await start_service()


@pytest.mark.asyncio
async def test_drain_called_in_finally_when_raw_videos_raises(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """nc.drain() is called in the finally block even when raw_videos raises"""
    nc, js = js_context
    drain_called = False

    async def spy_drain() -> None:
        nonlocal drain_called
        drain_called = True
        # Don't call the real drain — the connection is shared with the fixture

    nc.drain = spy_drain

    async def failing_raw_videos(_js: JetStreamContext) -> None:
        raise RuntimeError("subscriber failed unexpectedly")

    with (
        patch("src.service.nats_connect", return_value=(nc, js)),
        patch("src.service.raw_videos", side_effect=failing_raw_videos),
    ):
        with pytest.raises(RuntimeError, match="subscriber failed unexpectedly"):
            await start_service()

    assert drain_called


@pytest.mark.asyncio
async def test_drain_called_in_finally_on_cancellation(
    js_context: tuple[Any, JetStreamContext],
) -> None:
    """nc.drain() is called in the finally block when the service task is cancelled"""
    nc, js = js_context
    drain_called = False

    async def spy_drain() -> None:
        nonlocal drain_called
        drain_called = True
        # Don't call the real drain — the connection is shared with the fixture

    nc.drain = spy_drain

    async def hanging_raw_videos(_js: JetStreamContext) -> None:
        await asyncio.sleep(30)

    with (
        patch("src.service.nats_connect", return_value=(nc, js)),
        patch("src.service.raw_videos", side_effect=hanging_raw_videos),
    ):
        task = asyncio.create_task(start_service())
        await asyncio.sleep(0.05)
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass

    assert drain_called


@pytest.mark.asyncio
async def test_service_can_be_cancelled_while_process_job_is_running(
    js_context: tuple[Any, JetStreamContext],
    monkeypatch: Any,
) -> None:
    """Service cancels promptly mid-processing, proving process_job does not block the event loop"""
    nc, js = js_context
    monkeypatch.setattr(
        "src.nats.subscriber.settings.SCENE_SPLIT_SUBJECT", settings.SCENE_SPLIT_SUBJECT
    )
    # Unique consumer name: prevents unacked messages from leaking into other tests' durable consumers
    monkeypatch.setattr(
        "src.nats.subscriber.settings.NATS_SUB_QUEUE_NAME", "test-cancellation-worker"
    )
    # No-op drain: prevents start_service's finally block from closing the shared fixture connection
    nc.drain = AsyncMock()

    processing_started = asyncio.Event()

    async def slow_process_job(_metadata: Any) -> list[Any]:
        processing_started.set()
        await asyncio.sleep(30)
        return []

    with (
        patch("src.service.nats_connect", return_value=(nc, js)),
        patch("src.nats.subscriber.process_job", side_effect=slow_process_job),
    ):
        task = asyncio.create_task(start_service())
        payload = json.dumps(
            {
                "job_id": "1",
                "storage_path": "/fake/video.mp4",
                "target_resolution": "480p",
            }
        ).encode()
        await nc.publish(settings.SCENE_SPLIT_SUBJECT, payload)
        await processing_started.wait()

        task.cancel()
        try:
            await asyncio.wait_for(task, timeout=2.0)
        except asyncio.CancelledError:
            pass  # expected — task was properly cancelled
        except asyncio.TimeoutError:
            pytest.fail(
                "Service did not cancel within 2 seconds — process_job may be blocking the event loop"
            )
