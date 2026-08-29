from concurrent.futures import Future
from nats.aio.client import Client as NATSClient
from shared_handler import ProgressMessage
import asyncio


class ProgressReporter:
    """Publishes throttled progress updates for video_upscale to NATS,
    and lets the caller wait for all publishes to land before advancing stages."""

    def __init__(
        self,
        nc: NATSClient,
        job_id: str,
        loop: asyncio.AbstractEventLoop,
        service_name: str,
    ) -> None:
        self._nc = nc
        self._job_id = job_id
        self._loop = loop
        self._service_name = service_name
        self._last_progress = -1
        self._pending: list[Future[None]] = []

    def __call__(self, pct: int) -> None:
        """"""
        if pct == self._last_progress:
            return
        self._last_progress = pct
        fut = asyncio.run_coroutine_threadsafe(
            self._nc.publish(
                f"progress.{self._job_id}",
                ProgressMessage(
                    job_id=self._job_id,
                    stage=self._service_name,
                    progress=pct,
                )
                .model_dump_json()
                .encode(),
            ),
            self._loop,
        )
        self._pending.append(fut)

    async def flush(self, timeout_s: float = 10.0) -> None:
        """Await every progress publish scheduled since the last flush
        Bounded by timeout so stuck publish cant hang forever
        """
        futs = self._pending
        self._pending = []
        awaitables = [asyncio.wrap_future(fut) for fut in futs]
        try:
            await asyncio.wait_for(asyncio.gather(*awaitables), timeout=timeout_s)
        except asyncio.TimeoutError:
            for fut in futs:
                if not fut.done():
                    fut.cancel()
            raise
