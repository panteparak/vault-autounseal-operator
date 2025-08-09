import asyncio
import sys
from unittest.mock import patch

import pytest

# Configure pytest-asyncio
pytest_plugins = ("pytest_asyncio",)


@pytest.fixture(scope="session")
def event_loop():
    """Create an instance of the default event loop for the test session."""
    if sys.platform.startswith("win"):
        asyncio.set_event_loop_policy(asyncio.WindowsProactorEventLoopPolicy())
    loop = asyncio.get_event_loop_policy().new_event_loop()
    yield loop
    loop.close()


@pytest.fixture
def mock_kubernetes_config():
    """Mock Kubernetes configuration loading"""
    with (
        patch("kubernetes.config.load_incluster_config") as mock_incluster,
        patch("kubernetes.config.load_kube_config") as mock_kube,
    ):
        mock_incluster.side_effect = Exception("Not in cluster")
        mock_kube.return_value = None
        yield


@pytest.fixture
def mock_logging():
    """Mock logging to reduce test noise"""
    with (
        patch("vault_autounseal_operator.vault_client.logger"),
        patch("vault_autounseal_operator.operator_v2.logger"),
        patch("vault_autounseal_operator.security.logger"),
    ):
        yield
