from typing import Generator
from testcontainers.nats import NatsContainer
import pytest


@pytest.fixture(scope="session")
def setup_nats() -> Generator[NatsContainer, None]:
    """Start a nats"""
    with NatsContainer() as nats_container:
        client = nats_container(nats_container.nats_uri)
        yield client

@pytest.fixture(scope="session")
def nats_url() -> Generator[str, None]:
    """Starts a nats container and returns url"""
    with NatsContainer() as container:
        yield container.nats_uri()