from unittest.mock import AsyncMock, Mock, patch

import pytest
from kubernetes.client.rest import ApiException

from vault_autounseal_operator.pod_watcher import PodWatcher
from vault_autounseal_operator.vault_client import VaultClient


class TestPodWatcher:

    @pytest.fixture
    def mock_vault_client(self):
        mock_client = Mock(spec=VaultClient)
        mock_client.is_sealed = AsyncMock(return_value=True)
        mock_client.unseal = AsyncMock(return_value={"sealed": False})
        return mock_client

    @pytest.fixture
    def mock_k8s_client(self):
        with patch(
            "vault_autounseal_operator.pod_watcher.client.CoreV1Api"
        ) as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client
            yield mock_client

    @pytest.fixture
    def pod_watcher(self, mock_vault_client):
        return PodWatcher(
            namespace="vault",
            pod_selector={"matchLabels": {"app": "vault"}},
            vault_client=mock_vault_client,
            unseal_keys=["key1", "key2", "key3"],
            threshold=2,
        )

    def test_pod_watcher_initialization(self, mock_vault_client):
        watcher = PodWatcher(
            namespace="test-ns",
            pod_selector={"matchLabels": {"app": "vault", "role": "server"}},
            vault_client=mock_vault_client,
            unseal_keys=["key1", "key2"],
            threshold=2,
        )

        assert watcher.namespace == "test-ns"
        assert watcher.pod_selector == {
            "matchLabels": {"app": "vault", "role": "server"}
        }
        assert watcher.vault_client == mock_vault_client
        assert watcher.unseal_keys == ["key1", "key2"]
        assert watcher.threshold == 2
        assert not watcher.running
        assert watcher.monitored_pods == {}

    def test_matches_selector_exact_match(self, pod_watcher):
        pod_labels = {"app": "vault", "version": "1.0.0"}
        assert pod_watcher._matches_selector(pod_labels) is True

    def test_matches_selector_subset_match(self, pod_watcher):
        # Pod has the required label plus others
        pod_labels = {"app": "vault", "component": "server", "version": "1.0.0"}
        assert pod_watcher._matches_selector(pod_labels) is True

    def test_matches_selector_no_match(self, pod_watcher):
        pod_labels = {"app": "redis", "version": "1.0.0"}
        assert pod_watcher._matches_selector(pod_labels) is False

    def test_matches_selector_missing_label(self, pod_watcher):
        pod_labels = {"component": "server", "version": "1.0.0"}
        assert pod_watcher._matches_selector(pod_labels) is False

    def test_matches_selector_empty_selector(self, mock_vault_client):
        watcher = PodWatcher(
            namespace="vault",
            pod_selector={"matchLabels": {}},
            vault_client=mock_vault_client,
            unseal_keys=["key1"],
            threshold=1,
        )

        # Empty selector should match any pod
        assert watcher._matches_selector({"app": "vault"}) is True
        assert watcher._matches_selector({}) is True

    @pytest.mark.asyncio
    async def test_is_vault_pod_ready_and_sealed_running_and_sealed(
        self, pod_watcher, mock_vault_client
    ):
        mock_vault_client.is_sealed.return_value = True

        pod = {
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}, {"ready": True}],
            }
        }

        result = await pod_watcher._is_vault_pod_ready_and_sealed(pod)
        assert result is True
        mock_vault_client.is_sealed.assert_called_once()

    @pytest.mark.asyncio
    async def test_is_vault_pod_ready_and_sealed_not_running(
        self, pod_watcher, mock_vault_client
    ):
        pod = {
            "status": {
                "phase": "Pending",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            }
        }

        result = await pod_watcher._is_vault_pod_ready_and_sealed(pod)
        assert result is False
        mock_vault_client.is_sealed.assert_not_called()

    @pytest.mark.asyncio
    async def test_is_vault_pod_ready_and_sealed_no_ip(
        self, pod_watcher, mock_vault_client
    ):
        pod = {"status": {"phase": "Running", "containerStatuses": [{"ready": True}]}}

        result = await pod_watcher._is_vault_pod_ready_and_sealed(pod)
        assert result is False
        mock_vault_client.is_sealed.assert_not_called()

    @pytest.mark.asyncio
    async def test_is_vault_pod_ready_and_sealed_containers_not_ready(
        self, pod_watcher, mock_vault_client
    ):
        pod = {
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [
                    {"ready": True},
                    {"ready": False},  # One container not ready
                ],
            }
        }

        result = await pod_watcher._is_vault_pod_ready_and_sealed(pod)
        assert result is False
        mock_vault_client.is_sealed.assert_not_called()

    @pytest.mark.asyncio
    async def test_is_vault_pod_ready_and_sealed_unsealed(
        self, pod_watcher, mock_vault_client
    ):
        mock_vault_client.is_sealed.return_value = False

        pod = {
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            }
        }

        result = await pod_watcher._is_vault_pod_ready_and_sealed(pod)
        assert result is False
        mock_vault_client.is_sealed.assert_called_once()

    @pytest.mark.asyncio
    async def test_is_vault_pod_ready_and_sealed_vault_error(
        self, pod_watcher, mock_vault_client
    ):
        mock_vault_client.is_sealed.side_effect = Exception("Connection error")

        pod = {
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            }
        }

        result = await pod_watcher._is_vault_pod_ready_and_sealed(pod)
        assert result is False

    @pytest.mark.asyncio
    async def test_attempt_unseal_success(self, pod_watcher, mock_vault_client):
        mock_vault_client.unseal.return_value = {"sealed": False}

        await pod_watcher._attempt_unseal("test-pod")

        mock_vault_client.unseal.assert_called_once_with(["key1", "key2", "key3"], 2)

    @pytest.mark.asyncio
    async def test_attempt_unseal_failure(self, pod_watcher, mock_vault_client):
        mock_vault_client.unseal.side_effect = Exception("Unseal failed")

        # Should not raise exception, just log error
        await pod_watcher._attempt_unseal("test-pod")

        mock_vault_client.unseal.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_pod_event_added_sealed_pod(
        self, pod_watcher, mock_vault_client
    ):
        mock_vault_client.is_sealed.return_value = True
        mock_vault_client.unseal.return_value = {"sealed": False}

        pod = {
            "metadata": {
                "name": "vault-0",
                "uid": "uid-123",
                "labels": {"app": "vault"},
            },
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            },
        }

        await pod_watcher._handle_pod_event("ADDED", pod)

        # Pod should be added to monitored pods
        assert "uid-123" in pod_watcher.monitored_pods
        assert pod_watcher.monitored_pods["uid-123"]["name"] == "vault-0"

        # Should attempt to unseal
        mock_vault_client.unseal.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_pod_event_added_unsealed_pod(
        self, pod_watcher, mock_vault_client
    ):
        mock_vault_client.is_sealed.return_value = False

        pod = {
            "metadata": {
                "name": "vault-0",
                "uid": "uid-123",
                "labels": {"app": "vault"},
            },
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            },
        }

        await pod_watcher._handle_pod_event("ADDED", pod)

        # Pod should not be added to monitored pods
        assert "uid-123" not in pod_watcher.monitored_pods

        # Should not attempt to unseal
        mock_vault_client.unseal.assert_not_called()

    @pytest.mark.asyncio
    async def test_handle_pod_event_deleted(self, pod_watcher):
        # Add a pod to monitored pods first
        pod_watcher.monitored_pods["uid-123"] = {
            "name": "vault-0",
            "namespace": "vault",
            "last_unseal_attempt": 0,
        }

        pod = {
            "metadata": {
                "name": "vault-0",
                "uid": "uid-123",
                "labels": {"app": "vault"},
            }
        }

        await pod_watcher._handle_pod_event("DELETED", pod)

        # Pod should be removed from monitored pods
        assert "uid-123" not in pod_watcher.monitored_pods

    @pytest.mark.asyncio
    async def test_handle_pod_event_modified_now_unsealed(
        self, pod_watcher, mock_vault_client
    ):
        # Add a pod to monitored pods first
        pod_watcher.monitored_pods["uid-123"] = {
            "name": "vault-0",
            "namespace": "vault",
            "last_unseal_attempt": 0,
        }

        mock_vault_client.is_sealed.return_value = False

        pod = {
            "metadata": {
                "name": "vault-0",
                "uid": "uid-123",
                "labels": {"app": "vault"},
            },
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            },
        }

        await pod_watcher._handle_pod_event("MODIFIED", pod)

        # Pod should be removed from monitored pods since it's now unsealed
        assert "uid-123" not in pod_watcher.monitored_pods

    @pytest.mark.asyncio
    async def test_handle_pod_event_wrong_selector(
        self, pod_watcher, mock_vault_client
    ):
        pod = {
            "metadata": {
                "name": "redis-0",
                "uid": "uid-123",
                "labels": {"app": "redis"},  # Wrong label
            },
            "status": {
                "phase": "Running",
                "podIP": "10.0.0.1",
                "containerStatuses": [{"ready": True}],
            },
        }

        await pod_watcher._handle_pod_event("ADDED", pod)

        # Pod should not be processed
        assert "uid-123" not in pod_watcher.monitored_pods
        mock_vault_client.is_sealed.assert_not_called()

    @pytest.mark.asyncio
    async def test_start_and_stop(self, pod_watcher):
        assert not pod_watcher.running

        with patch.object(
            pod_watcher, "_watch_pods", new_callable=AsyncMock
        ) as mock_watch:
            # Start watcher
            await pod_watcher.start()
            assert pod_watcher.running
            assert pod_watcher.watch_task is not None

            # Stop watcher
            await pod_watcher.stop()
            assert not pod_watcher.running

            # Watch task should have been cancelled
            mock_watch.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_monitored_pods(self, pod_watcher):
        # Add some test pods
        pod_watcher.monitored_pods = {
            "uid-1": {"name": "vault-0", "namespace": "vault"},
            "uid-2": {"name": "vault-1", "namespace": "vault"},
        }

        result = await pod_watcher.get_monitored_pods()

        assert len(result) == 2
        assert {"name": "vault-0", "namespace": "vault"} in result
        assert {"name": "vault-1", "namespace": "vault"} in result

    @pytest.mark.asyncio
    async def test_watch_pods_api_error_410(self, pod_watcher, mock_k8s_client):
        """Test that 410 errors (resource expired) are handled gracefully"""

        mock_stream = Mock()
        mock_stream.__iter__ = Mock(side_effect=ApiException(status=410))

        with patch(
            "vault_autounseal_operator.pod_watcher.watch.Watch"
        ) as mock_watch_class:
            mock_watch = Mock()
            mock_watch.stream.return_value = mock_stream
            mock_watch_class.return_value = mock_watch

            # Simulate the watch loop running briefly then stopping
            pod_watcher.running = True

            # Mock the method to stop after first iteration
            original_watch = pod_watcher._watch_pods
            call_count = 0

            async def mock_watch_with_stop(*args, **kwargs):
                nonlocal call_count
                call_count += 1
                if call_count > 1:  # Stop after first retry
                    pod_watcher.running = False
                await original_watch(*args, **kwargs)

            pod_watcher._watch_pods = mock_watch_with_stop

            # Should not raise exception, should continue watching
            await pod_watcher._watch_pods()

    @pytest.mark.asyncio
    async def test_watch_pods_unexpected_error(self, pod_watcher, mock_k8s_client):
        """Test handling of unexpected API errors"""

        mock_stream = Mock()
        mock_stream.__iter__ = Mock(side_effect=ApiException(status=500))

        with patch(
            "vault_autounseal_operator.pod_watcher.watch.Watch"
        ) as mock_watch_class:
            mock_watch = Mock()
            mock_watch.stream.return_value = mock_stream
            mock_watch_class.return_value = mock_watch

            pod_watcher.running = True

            # Mock sleep to speed up test
            with patch(
                "vault_autounseal_operator.pod_watcher.asyncio.sleep",
                new_callable=AsyncMock,
            ) as mock_sleep:
                # Stop after first error to prevent infinite loop
                async def stop_after_error(*args):
                    pod_watcher.running = False

                mock_sleep.side_effect = stop_after_error

                await pod_watcher._watch_pods()

                # Should have slept (error backoff)
                mock_sleep.assert_called_once_with(5)
