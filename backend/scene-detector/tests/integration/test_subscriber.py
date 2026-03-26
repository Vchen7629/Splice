from typing import AsyncGenerator
from unittest.mock import patch
from nats.js.client import JetStreamContext
from src.nats.subscriber import raw_videos
from src.nats.messages import SceneSplitMessage
import json
import pytest
import asyncio

@pytest.mark.asyncio
async def test_processes_published_message(js_context: AsyncGenerator[JetStreamContext, None], monkeypatch) -> None:
    """Verifies subscriber receives a message and calls process_job with correct data"""                                                                     
    nc, js = js_context                                                                                                                                      
    monkeypatch.setattr("src.nats.subscriber.settings.SCENE_SPLIT_SUBJECT", "jobs.video.scene-split")                                                        
    monkeypatch.setattr("src.nats.subscriber.settings.NATS_SUB_QUEUE_NAME", "scene-detector-workers")                                                        
                                                                                                                                                            
    payload = json.dumps({"job_id": "1", "storage_path": "/fake/video.mp4"}).encode()                                                                        
    received = []                                                                                                                                            
                                                                                                                                                            
    async def fake_process_job(metadata, js):
        received.append(metadata)

    with patch("src.nats.subscriber.process_job", side_effect=fake_process_job):                                                                             
        task = asyncio.create_task(raw_videos(js))
        await nc.publish("jobs.video.scene-split", payload)                                                                                                  
        await asyncio.sleep(0.5)  # let the subscriber process the message                                                                                   
        task.cancel()
        try:                                                                                                                                                 
            await task
        except asyncio.CancelledError:                                                                                                                       
            pass

    assert len(received) == 1
    assert received[0] == SceneSplitMessage(job_id="1", storage_path="/fake/video.mp4")