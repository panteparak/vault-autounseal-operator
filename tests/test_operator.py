import base64
import json
from unittest.mock import AsyncMock, Mock, patch

import pytest
from kubernetes.client.rest import ApiException

from vault_autounseal_operator.models import VaultStatus
from vault_autounseal_operator.operator_v2 import (
    VaultOperatorV2,
    create_vault_config,
    delete_vault_config,
    update_vault_config,
)


class TestVaultOperatorV2:

    @pytest.fixture
    def operator(self):
        return VaultOperatorV2()

    @pytest.fixture
    def mock_k8s_client(self, operator):
        with patch(
            "vault_autounseal_operator.operator_v2.client.CoreV1Api"
        ) as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client
            operator.k8s_client = mock_client
            yield mock_client

    @pytest.fixture
    def mock_vault_client(self):
        with patch(
            "vault_autounseal_operator.operator_v2.VaultClient"
        ) as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client
            yield mock_client

    @pytest.fixture
    def mock_pod_watcher(self):
        with patch(
            "vault_autounseal_operator.operator_v2.PodWatcher"
        ) as mock_watcher_class:
            mock_watcher = Mock()
            mock_watcher.start = AsyncMock()
            mock_watcher.stop = AsyncMock()
            mock_watcher_class.return_value = mock_watcher
            yield mock_watcher

    @pytest.mark.asyncio
    async def test_get_unseal_keys_direct_secret(self, operator, mock_k8s_client):
        spec = {"unsealKeys": {"secret": ["key1", "key2", "key3"]}}

        keys = await operator.get_unseal_keys(spec, "default")
        assert keys == ["key1", "key2", "key3"]

        # Should not call Kubernetes API for direct secrets
        mock_k8s_client.read_namespaced_secret.assert_not_called()

    @pytest.mark.asyncio
    async def test_get_unseal_keys_secret_ref_json(self, operator, mock_k8s_client):
        # Mock secret data as JSON array
        keys_json = json.dumps(["key1", "key2", "key3"])
        encoded_keys = base64.b64encode(keys_json.encode()).decode()

        mock_secret = Mock()
        mock_secret.data = {"unseal-keys": encoded_keys}
        mock_k8s_client.read_namespaced_secret.return_value = mock_secret

        spec = {
            "unsealKeys": {"secretRef": {"name": "vault-keys", "key": "unseal-keys"}}
        }

        keys = await operator.get_unseal_keys(spec, "default")
        assert keys == ["key1", "key2", "key3"]

        mock_k8s_client.read_namespaced_secret.assert_called_once_with(
            name="vault-keys", namespace="default"
        )

    @pytest.mark.asyncio
    async def test_get_unseal_keys_secret_ref_newline_separated(
        self, operator, mock_k8s_client
    ):
        # Mock secret data as newline-separated keys
        keys_data = "key1\nkey2\nkey3\n"
        encoded_keys = base64.b64encode(keys_data.encode()).decode()

        mock_secret = Mock()
        mock_secret.data = {"keys": encoded_keys}
        mock_k8s_client.read_namespaced_secret.return_value = mock_secret

        spec = {
            "unsealKeys": {
                "secretRef": {"name": "vault-keys", "namespace": "vault", "key": "keys"}
            }
        }

        keys = await operator.get_unseal_keys(spec, "default")
        assert keys == ["key1", "key2", "key3"]

        mock_k8s_client.read_namespaced_secret.assert_called_once_with(
            name="vault-keys", namespace="vault"
        )

    @pytest.mark.asyncio
    async def test_get_unseal_keys_secret_not_found(self, operator, mock_k8s_client):
        mock_k8s_client.read_namespaced_secret.side_effect = ApiException(status=404)

        spec = {"unsealKeys": {"secretRef": {"name": "nonexistent-secret"}}}

        with pytest.raises(ValueError, match="Failed to read secret"):
            await operator.get_unseal_keys(spec, "default")

    @pytest.mark.asyncio
    async def test_get_unseal_keys_missing_key_in_secret(
        self, operator, mock_k8s_client
    ):
        mock_secret = Mock()
        mock_secret.data = {"other-key": "data"}
        mock_k8s_client.read_namespaced_secret.return_value = mock_secret

        spec = {
            "unsealKeys": {"secretRef": {"name": "vault-keys", "key": "missing-key"}}
        }

        with pytest.raises(ValueError, match="Key 'missing-key' not found"):
            await operator.get_unseal_keys(spec, "default")

    @pytest.mark.asyncio
    async def test_get_unseal_keys_no_keys_specified(self, operator):
        spec = {"unsealKeys": {}}

        with pytest.raises(ValueError, match="No unseal keys provided"):
            await operator.get_unseal_keys(spec, "default")

    @pytest.mark.asyncio
    async def test_setup_vault_client_basic(
        self, operator, mock_vault_client, mock_pod_watcher
    ):
        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {"secret": ["key1"]},
            "tlsSkipVerify": True,
        }

        await operator.setup_vault_client(spec, "default", "test-vault")

        # Check vault client was created
        client_key = "default/test-vault"
        assert client_key in operator.vault_clients

        # Verify VaultClient was instantiated with correct parameters
        mock_vault_client.assert_called_once()
        # Note: We can't easily verify the exact parameters due to mocking complexity

    @pytest.mark.asyncio
    async def test_setup_vault_client_with_ha(
        self, operator, mock_vault_client, mock_pod_watcher, mock_k8s_client
    ):
        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {"secret": ["key1", "key2", "key3"]},
            "haEnabled": True,
            "namespace": "vault",
            "podSelector": {"matchLabels": {"app": "vault"}},
            "threshold": 2,
        }

        await operator.setup_vault_client(spec, "default", "test-vault")

        # Check both vault client and pod watcher were created
        client_key = "default/test-vault"
        assert client_key in operator.vault_clients
        assert client_key in operator.pod_watchers

        # Verify pod watcher was started
        mock_pod_watcher.start.assert_called_once()

    @pytest.mark.asyncio
    async def test_cleanup_vault_instance(
        self, operator, mock_vault_client, mock_pod_watcher
    ):
        # Setup some instances first
        client_key = "default/test-vault"
        operator.vault_clients[client_key] = mock_vault_client
        operator.pod_watchers[client_key] = mock_pod_watcher

        await operator.cleanup_vault_instance("default", "test-vault")

        # Verify cleanup
        assert client_key not in operator.vault_clients
        assert client_key not in operator.pod_watchers
        mock_pod_watcher.stop.assert_called_once()

    @pytest.mark.asyncio
    async def test_check_and_unseal_vault_already_unsealed(
        self, operator, mock_vault_client, mock_k8s_client
    ):
        # Mock vault client methods
        mock_vault_client.is_sealed = AsyncMock(return_value=False)

        client_key = "default/test-vault"
        operator.vault_clients[client_key] = mock_vault_client

        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {"secret": ["key1"]},
        }

        result = await operator.check_and_unseal_vault(spec, "default", "test-vault")

        assert isinstance(result, VaultStatus)
        assert result.sealed is False
        assert result.error is None
        assert result.lastUnsealed is not None

        # Should not attempt to unseal
        mock_vault_client.unseal.assert_not_called()

    @pytest.mark.asyncio
    async def test_check_and_unseal_vault_sealed(
        self, operator, mock_vault_client, mock_k8s_client
    ):
        # Mock vault client methods
        mock_vault_client.is_sealed = AsyncMock(return_value=True)
        mock_vault_client.unseal = AsyncMock(return_value={"sealed": False})

        client_key = "default/test-vault"
        operator.vault_clients[client_key] = mock_vault_client

        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {"secret": ["key1", "key2", "key3"]},
            "threshold": 2,
        }

        result = await operator.check_and_unseal_vault(spec, "default", "test-vault")

        assert isinstance(result, VaultStatus)
        assert result.sealed is False
        assert result.error is None

        # Should attempt to unseal
        mock_vault_client.unseal.assert_called_once_with(["key1", "key2", "key3"], 2)

    @pytest.mark.asyncio
    async def test_check_and_unseal_vault_error(
        self, operator, mock_vault_client, mock_k8s_client
    ):
        # Mock vault client to raise exception
        mock_vault_client.is_sealed = AsyncMock(
            side_effect=Exception("Connection error")
        )

        client_key = "default/test-vault"
        operator.vault_clients[client_key] = mock_vault_client

        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {"secret": ["key1"]},
        }

        result = await operator.check_and_unseal_vault(spec, "default", "test-vault")

        assert isinstance(result, VaultStatus)
        assert result.sealed is True
        assert result.error == "Connection error"


class TestOperatorHandlers:

    @pytest.fixture
    def mock_operator(self):
        with patch("vault_autounseal_operator.operator_v2.operator") as mock_op:
            mock_op.setup_vault_client = AsyncMock()
            mock_op.check_and_unseal_vault = AsyncMock()
            mock_op.cleanup_vault_instance = AsyncMock()
            yield mock_op

    @pytest.fixture
    def mock_security_validator(self):
        with patch(
            "vault_autounseal_operator.operator_v2.SecurityValidator"
        ) as mock_validator:
            mock_validator.validate_spec.return_value = {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ["key1"]},
                "threshold": 1,
                "namespace": "default",
                "haEnabled": False,
                "tlsSkipVerify": False,
                "reconcileInterval": "30s",
            }
            mock_validator.sanitize_log_data.return_value = {
                "url": "https://vault.example.com:8200"
            }
            yield mock_validator

    @pytest.mark.asyncio
    async def test_create_vault_config_success(
        self, mock_operator, mock_security_validator
    ):
        # Mock successful vault status
        mock_vault_status = VaultStatus(
            sealed=False,
            lastUnsealed="2023-01-01T00:00:00Z",
            lastChecked="2023-01-01T00:00:00Z",
            error=None,
        )
        mock_operator.check_and_unseal_vault.return_value = mock_vault_status

        body = {
            "spec": {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ["key1"]},
            }
        }

        result = await create_vault_config(body, "test-vault", "default")

        # Verify result structure
        assert "conditions" in result
        assert "vaultStatus" in result
        assert len(result["conditions"]) == 1

        condition = result["conditions"][0]
        assert condition["type"] == "Ready"
        assert condition["status"] == "True"
        assert condition["reason"] == "ConfigurationApplied"

        vault_status = result["vaultStatus"]
        assert vault_status["sealed"] is False
        assert vault_status["error"] is None

        # Verify operator methods were called
        mock_security_validator.validate_spec.assert_called_once()
        mock_operator.setup_vault_client.assert_called_once()
        mock_operator.check_and_unseal_vault.assert_called_once()

    @pytest.mark.asyncio
    async def test_create_vault_config_validation_error(
        self, mock_operator, mock_security_validator
    ):
        mock_security_validator.validate_spec.side_effect = ValueError("Invalid URL")

        body = {"spec": {"url": "invalid-url", "unsealKeys": {"secret": ["key1"]}}}

        result = await create_vault_config(body, "test-vault", "default")

        # Should return error condition
        assert "conditions" in result
        condition = result["conditions"][0]
        assert condition["type"] == "Ready"
        assert condition["status"] == "False"
        assert condition["reason"] == "Error"
        assert "Invalid URL" in condition["message"]

        # Should not call operator methods after validation failure
        mock_operator.setup_vault_client.assert_not_called()

    @pytest.mark.asyncio
    async def test_create_vault_config_vault_error(
        self, mock_operator, mock_security_validator
    ):
        # Mock vault status with error
        mock_vault_status = VaultStatus(
            sealed=True,
            lastUnsealed=None,
            lastChecked="2023-01-01T00:00:00Z",
            error="Connection failed",
        )
        mock_operator.check_and_unseal_vault.return_value = mock_vault_status

        body = {
            "spec": {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ["key1"]},
            }
        }

        result = await create_vault_config(body, "test-vault", "default")

        # Should return error condition
        condition = result["conditions"][0]
        assert condition["status"] == "False"
        assert condition["reason"] == "Error"

        # Should still include vault status
        vault_status = result["vaultStatus"]
        assert vault_status["sealed"] is True
        assert vault_status["error"] == "Connection failed"

    @pytest.mark.asyncio
    async def test_update_vault_config(self, mock_operator, mock_security_validator):
        mock_vault_status = VaultStatus(
            sealed=False,
            lastUnsealed="2023-01-01T00:00:00Z",
            lastChecked="2023-01-01T00:00:00Z",
            error=None,
        )
        mock_operator.check_and_unseal_vault.return_value = mock_vault_status

        body = {
            "spec": {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": ["key1"]},
            }
        }
        old = {"spec": {"url": "https://old-vault.example.com:8200"}}
        new = body

        result = await update_vault_config(body, "test-vault", "default", old, new)

        # Should cleanup old and create new
        mock_operator.cleanup_vault_instance.assert_called_once_with(
            "default", "test-vault"
        )
        mock_operator.setup_vault_client.assert_called_once()

        # Should return success result
        assert result["conditions"][0]["reason"] == "ConfigurationApplied"

    @pytest.mark.asyncio
    async def test_delete_vault_config(self, mock_operator):
        body = {"spec": {"url": "https://vault.example.com:8200"}}

        await delete_vault_config(body, "test-vault", "default")

        mock_operator.cleanup_vault_instance.assert_called_once_with(
            "default", "test-vault"
        )
