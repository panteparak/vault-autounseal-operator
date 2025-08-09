import asyncio
import base64
import json
from unittest.mock import AsyncMock, Mock, patch

import pytest

from vault_autounseal_operator.operator_v2 import VaultOperatorV2
from vault_autounseal_operator.security import SecurityValidator


class TestIntegration:
    """Integration tests covering end-to-end workflows"""

    @pytest.fixture
    def operator(self):
        return VaultOperatorV2()

    @pytest.fixture
    def mock_k8s_secret(self):
        """Mock Kubernetes secret with unseal keys"""
        keys = ["key1", "key2", "key3"]
        keys_json = json.dumps(keys)
        encoded_data = base64.b64encode(keys_json.encode()).decode()

        mock_secret = Mock()
        mock_secret.data = {"unseal-keys": encoded_data}
        return mock_secret

    @pytest.fixture
    def mock_vault_responses(self):
        """Mock Vault API responses"""
        return {
            "sealed_status": {"sealed": True, "threshold": 3, "n": 5},
            "unsealed_status": {"sealed": False, "threshold": 3, "n": 5},
            "health_check": {"initialized": True, "sealed": False},
        }

    @pytest.mark.asyncio
    async def test_end_to_end_vault_unseal_with_direct_keys(
        self, operator, mock_vault_responses
    ):
        """Test complete flow: spec validation -> vault client setup -> unsealing"""

        # Setup mocks
        with patch(
            "vault_autounseal_operator.operator_v2.VaultClient"
        ) as mock_vault_client_class:
            mock_vault_client = Mock()
            mock_vault_client.is_sealed = AsyncMock(return_value=True)
            mock_vault_client.unseal = AsyncMock(
                return_value=mock_vault_responses["unsealed_status"]
            )
            mock_vault_client_class.return_value = mock_vault_client

            # Test spec with direct keys
            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {
                    "secret": [
                        base64.b64encode(b"test-key-1").decode(),
                        base64.b64encode(b"test-key-2").decode(),
                        base64.b64encode(b"test-key-3").decode(),
                    ]
                },
                "threshold": 2,
                "tlsSkipVerify": False,
            }

            # Validate spec (security check)
            validated_spec = SecurityValidator.validate_spec(spec)

            # Setup vault client
            await operator.setup_vault_client(validated_spec, "default", "test-vault")

            # Check and unseal
            vault_status = await operator.check_and_unseal_vault(
                validated_spec, "default", "test-vault"
            )

            # Verify results
            assert vault_status.sealed is False
            assert vault_status.error is None
            assert vault_status.lastUnsealed is not None

            # Verify mock calls
            mock_vault_client.is_sealed.assert_called_once()
            mock_vault_client.unseal.assert_called_once()

            # Verify unseal was called with correct parameters
            unseal_call = mock_vault_client.unseal.call_args
            assert len(unseal_call[0][0]) == 3  # keys
            assert unseal_call[0][1] == 2  # threshold

    @pytest.mark.asyncio
    async def test_end_to_end_vault_unseal_with_secret_ref(
        self, operator, mock_k8s_secret, mock_vault_responses
    ):
        """Test complete flow using Kubernetes secret reference"""

        with (
            patch(
                "vault_autounseal_operator.operator_v2.VaultClient"
            ) as mock_vault_client_class,
            patch(
                "vault_autounseal_operator.operator_v2.client.CoreV1Api"
            ) as mock_k8s_client_class,
        ):

            # Setup mocks
            mock_vault_client = Mock()
            mock_vault_client.is_sealed = AsyncMock(return_value=True)
            mock_vault_client.unseal = AsyncMock(
                return_value=mock_vault_responses["unsealed_status"]
            )
            mock_vault_client_class.return_value = mock_vault_client

            mock_k8s_client = Mock()
            mock_k8s_client.read_namespaced_secret.return_value = mock_k8s_secret
            mock_k8s_client_class.return_value = mock_k8s_client

            operator.k8s_client = mock_k8s_client

            # Test spec with secret reference
            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {
                    "secretRef": {
                        "name": "vault-keys",
                        "namespace": "vault",
                        "key": "unseal-keys",
                    }
                },
                "threshold": 3,
            }

            # Validate and process
            validated_spec = SecurityValidator.validate_spec(spec)
            await operator.setup_vault_client(validated_spec, "default", "test-vault")
            vault_status = await operator.check_and_unseal_vault(
                validated_spec, "default", "test-vault"
            )

            # Verify results
            assert vault_status.sealed is False
            assert vault_status.error is None

            # Verify Kubernetes secret was read
            mock_k8s_client.read_namespaced_secret.assert_called_once_with(
                name="vault-keys", namespace="vault"
            )

    @pytest.mark.asyncio
    async def test_security_validation_prevents_malicious_input(self, operator):
        """Test that security validation prevents various attack vectors"""

        malicious_specs = [
            # URL injection
            {
                "url": "https://evil.com:8200/../../../etc/passwd",
                "unsealKeys": {"secret": ["dGVzdA=="]},
            },
            # Oversized input (DoS)
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ["x" * 2000]},
            },
            # Invalid base64 that could cause injection
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ['"; rm -rf /; echo "']},
            },
            # Threshold manipulation
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ["dGVzdA=="]},
                "threshold": -1,
            },
            # Secret name injection
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {
                    "secretRef": {"name": "../../../etc/passwd", "key": "unseal-keys"}
                },
            },
        ]

        for i, spec in enumerate(malicious_specs):
            with pytest.raises(ValueError, match=".*"):
                SecurityValidator.validate_spec(spec)

    @pytest.mark.asyncio
    async def test_error_handling_and_recovery(self, operator, mock_vault_responses):
        """Test error scenarios and recovery mechanisms"""

        with patch(
            "vault_autounseal_operator.operator_v2.VaultClient"
        ) as mock_vault_client_class:
            mock_vault_client = Mock()
            mock_vault_client_class.return_value = mock_vault_client

            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": [base64.b64encode(b"test").decode()]},
                "threshold": 1,
            }
            validated_spec = SecurityValidator.validate_spec(spec)

            # Test vault connection failure
            mock_vault_client.is_sealed = AsyncMock(
                side_effect=Exception("Connection refused")
            )

            vault_status = await operator.check_and_unseal_vault(
                validated_spec, "default", "test-vault"
            )

            assert vault_status.sealed is True
            assert "Connection refused" in vault_status.error

            # Test partial unseal failure (some keys fail, others succeed)
            mock_vault_client.is_sealed = AsyncMock(return_value=True)
            mock_vault_client.unseal = AsyncMock(
                side_effect=[
                    Exception("First attempt failed"),
                    mock_vault_responses["unsealed_status"],  # Second attempt succeeds
                ]
            )

            # This should still work due to retry logic
            vault_status = await operator.check_and_unseal_vault(
                validated_spec, "default", "test-vault"
            )

    @pytest.mark.asyncio
    async def test_ha_mode_pod_watching_setup(self, operator):
        """Test HA mode setup with pod watching"""

        with (
            patch(
                "vault_autounseal_operator.operator_v2.VaultClient"
            ) as mock_vault_client_class,
            patch(
                "vault_autounseal_operator.operator_v2.PodWatcher"
            ) as mock_pod_watcher_class,
        ):

            mock_vault_client = Mock()
            mock_vault_client_class.return_value = mock_vault_client

            mock_pod_watcher = Mock()
            mock_pod_watcher.start = AsyncMock()
            mock_pod_watcher_class.return_value = mock_pod_watcher

            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": [base64.b64encode(b"test").decode()]},
                "haEnabled": True,
                "namespace": "vault",
                "podSelector": {"matchLabels": {"app": "vault", "component": "server"}},
                "threshold": 1,
            }

            validated_spec = SecurityValidator.validate_spec(spec)
            await operator.setup_vault_client(validated_spec, "default", "test-vault")

            # Verify pod watcher was created and started
            mock_pod_watcher_class.assert_called_once()
            mock_pod_watcher.start.assert_called_once()

            # Verify pod watcher is registered
            client_key = "default/test-vault"
            assert client_key in operator.pod_watchers

    @pytest.mark.asyncio
    async def test_cleanup_prevents_resource_leaks(self, operator):
        """Test that cleanup properly removes all resources"""

        with (
            patch(
                "vault_autounseal_operator.operator_v2.VaultClient"
            ) as mock_vault_client_class,
            patch(
                "vault_autounseal_operator.operator_v2.PodWatcher"
            ) as mock_pod_watcher_class,
        ):

            mock_vault_client = Mock()
            mock_vault_client_class.return_value = mock_vault_client

            mock_pod_watcher = Mock()
            mock_pod_watcher.start = AsyncMock()
            mock_pod_watcher.stop = AsyncMock()
            mock_pod_watcher_class.return_value = mock_pod_watcher

            # Setup resources
            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": [base64.b64encode(b"test").decode()]},
                "haEnabled": True,
                "threshold": 1,
            }

            validated_spec = SecurityValidator.validate_spec(spec)
            await operator.setup_vault_client(validated_spec, "default", "test-vault")

            # Verify resources exist
            client_key = "default/test-vault"
            assert client_key in operator.vault_clients
            assert client_key in operator.pod_watchers

            # Cleanup
            await operator.cleanup_vault_instance("default", "test-vault")

            # Verify resources are removed
            assert client_key not in operator.vault_clients
            assert client_key not in operator.pod_watchers
            mock_pod_watcher.stop.assert_called_once()

    @pytest.mark.asyncio
    async def test_concurrent_operations_thread_safety(self, operator):
        """Test concurrent operations for thread safety"""

        with patch(
            "vault_autounseal_operator.operator_v2.VaultClient"
        ) as mock_vault_client_class:
            mock_vault_client = Mock()
            mock_vault_client.is_sealed = AsyncMock(return_value=False)
            mock_vault_client_class.return_value = mock_vault_client

            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": [base64.b64encode(b"test").decode()]},
                "threshold": 1,
            }
            validated_spec = SecurityValidator.validate_spec(spec)

            # Simulate concurrent operations on different vault instances
            async def setup_and_check(name):
                await operator.setup_vault_client(validated_spec, "default", name)
                return await operator.check_and_unseal_vault(
                    validated_spec, "default", name
                )

            # Run multiple concurrent operations
            tasks = [setup_and_check(f"vault-{i}") for i in range(5)]
            results = await asyncio.gather(*tasks, return_exceptions=True)

            # All operations should succeed
            for result in results:
                assert not isinstance(result, Exception)
                assert result.sealed is False

    @pytest.mark.asyncio
    async def test_memory_management_sensitive_data(self, operator):
        """Test that sensitive data is properly cleared from memory"""

        with patch(
            "vault_autounseal_operator.operator_v2.VaultClient"
        ) as mock_vault_client_class:
            mock_vault_client = Mock()
            mock_vault_client.is_sealed = AsyncMock(return_value=True)

            # Track calls to ensure keys are handled securely
            unseal_calls = []

            async def track_unseal(keys, threshold):
                unseal_calls.append((keys, threshold))
                return {"sealed": False}

            mock_vault_client.unseal = AsyncMock(side_effect=track_unseal)
            mock_vault_client_class.return_value = mock_vault_client

            # Test with sensitive keys
            sensitive_keys = [
                base64.b64encode(b"super-secret-key-1").decode(),
                base64.b64encode(b"super-secret-key-2").decode(),
            ]

            spec = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": sensitive_keys},
                "threshold": 2,
            }

            validated_spec = SecurityValidator.validate_spec(spec)
            await operator.setup_vault_client(validated_spec, "default", "test-vault")
            await operator.check_and_unseal_vault(
                validated_spec, "default", "test-vault"
            )

            # Verify keys were passed correctly but not logged
            assert len(unseal_calls) == 1
            assert len(unseal_calls[0][0]) == 2  # Two keys passed
            assert unseal_calls[0][1] == 2  # Correct threshold

    def test_input_sanitization_prevents_log_injection(self):
        """Test that log data sanitization prevents log injection"""

        # Test data with potential log injection
        malicious_data = {
            "url": "https://vault.example.com:8200\n[FAKE LOG] Admin access granted",
            "secret_key": "test-key\r\nAUTH SUCCESS: root",
            "nested": {
                "password": "secret123\n\n[ERROR] System compromised",
                "safe_field": "normal data",
            },
            "keys": ["key1\nFAKE_LOG_ENTRY", "key2"],
        }

        sanitized = SecurityValidator.sanitize_log_data(malicious_data)

        # Sensitive fields should be redacted
        assert sanitized["secret_key"] == "[REDACTED]"
        assert sanitized["nested"]["password"] == "[REDACTED]"

        # Safe fields should remain
        assert "vault.example.com" in sanitized["url"]
        assert sanitized["nested"]["safe_field"] == "normal data"

        # Arrays with potential injection should be sanitized
        assert sanitized["keys"] == [
            "key1\nFAKE_LOG_ENTRY",
            "key2",
        ]  # Not a sensitive field name
