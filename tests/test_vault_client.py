import pytest
import asyncio
from unittest.mock import Mock, patch, AsyncMock
import base64
from vault_autounseal_operator.vault_client import VaultClient


class TestVaultClient:
    
    @pytest.fixture
    def mock_hvac_client(self):
        with patch('vault_autounseal_operator.vault_client.hvac.Client') as mock_client:
            yield mock_client.return_value
    
    @pytest.fixture
    def vault_client(self, mock_hvac_client):
        return VaultClient("https://vault.example.com:8200")
    
    def test_init_valid_url(self, mock_hvac_client):
        client = VaultClient("https://vault.example.com:8200")
        assert client.url == "https://vault.example.com:8200"
        assert client.timeout == 30
    
    def test_init_invalid_url(self):
        with pytest.raises(ValueError, match="Invalid URL format"):
            VaultClient("invalid-url")
    
    def test_init_tls_skip_verify(self, mock_hvac_client):
        with patch('vault_autounseal_operator.vault_client.urllib3') as mock_urllib3:
            client = VaultClient("https://vault.example.com:8200", tls_skip_verify=True)
            mock_urllib3.disable_warnings.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_is_sealed_true(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.return_value = {'sealed': True}
        
        result = await vault_client.is_sealed()
        assert result is True
    
    @pytest.mark.asyncio
    async def test_is_sealed_false(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.return_value = {'sealed': False}
        
        result = await vault_client.is_sealed()
        assert result is False
    
    @pytest.mark.asyncio
    async def test_is_sealed_exception(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.side_effect = Exception("Connection error")
        
        with pytest.raises(Exception, match="Connection error"):
            await vault_client.is_sealed()
    
    @pytest.mark.asyncio
    async def test_get_seal_status(self, vault_client, mock_hvac_client):
        expected_status = {'sealed': True, 'threshold': 3, 'n': 5}
        mock_hvac_client.sys.read_seal_status.return_value = expected_status
        
        result = await vault_client.get_seal_status()
        assert result == expected_status
    
    @pytest.mark.asyncio
    async def test_unseal_already_unsealed(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.return_value = {'sealed': False}
        
        keys = [base64.b64encode(b"key1").decode('ascii')]
        result = await vault_client.unseal(keys, 1)
        
        assert result == {'sealed': False}
        mock_hvac_client.sys.submit_unseal_key.assert_not_called()
    
    @pytest.mark.asyncio
    async def test_unseal_success(self, vault_client, mock_hvac_client):
        # First call: sealed, second call: unsealed after key submission
        mock_hvac_client.sys.read_seal_status.side_effect = [
            {'sealed': True},
            {'sealed': False}
        ]
        mock_hvac_client.sys.submit_unseal_key.return_value = {'sealed': False}
        
        keys = [base64.b64encode(b"key1").decode('ascii')]
        result = await vault_client.unseal(keys, 1)
        
        assert result == {'sealed': False}
        mock_hvac_client.sys.submit_unseal_key.assert_called_once_with("key1")
    
    @pytest.mark.asyncio
    async def test_unseal_multiple_keys(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.side_effect = [
            {'sealed': True},  # Initial status
            {'sealed': False}  # Final status after keys
        ]
        # First key doesn't unseal, second key unseals
        mock_hvac_client.sys.submit_unseal_key.side_effect = [
            {'sealed': True},
            {'sealed': False}
        ]
        
        keys = [
            base64.b64encode(b"key1").decode('ascii'),
            base64.b64encode(b"key2").decode('ascii')
        ]
        result = await vault_client.unseal(keys, 2)
        
        assert result == {'sealed': False}
        assert mock_hvac_client.sys.submit_unseal_key.call_count == 2
    
    @pytest.mark.asyncio
    async def test_unseal_invalid_inputs(self, vault_client):
        # Empty keys
        with pytest.raises(ValueError, match="No unseal keys provided"):
            await vault_client.unseal([], 1)
        
        # Invalid threshold
        keys = [base64.b64encode(b"key1").decode('ascii')]
        with pytest.raises(ValueError, match="Threshold must be at least 1"):
            await vault_client.unseal(keys, 0)
        
        with pytest.raises(ValueError, match="Threshold exceeds number of available keys"):
            await vault_client.unseal(keys, 2)
    
    @pytest.mark.asyncio
    async def test_unseal_invalid_base64_key(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.return_value = {'sealed': True}
        
        with pytest.raises(ValueError, match="is not valid base64"):
            await vault_client.unseal(["invalid-base64!"], 1)
    
    @pytest.mark.asyncio
    async def test_unseal_key_submission_error(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.side_effect = [
            {'sealed': True},  # Initial
            {'sealed': True}   # Still sealed after error
        ]
        mock_hvac_client.sys.submit_unseal_key.side_effect = Exception("Key error")
        
        keys = [base64.b64encode(b"key1").decode('ascii')]
        result = await vault_client.unseal(keys, 1)
        
        # Should return final status even if key submission failed
        assert result == {'sealed': True}
    
    @pytest.mark.asyncio
    async def test_unseal_partial_failure(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_seal_status.side_effect = [
            {'sealed': True},  # Initial
            {'sealed': False}  # Final - unsealed by second key
        ]
        # First key fails, second succeeds
        mock_hvac_client.sys.submit_unseal_key.side_effect = [
            Exception("First key error"),
            {'sealed': False}
        ]
        
        keys = [
            base64.b64encode(b"key1").decode('ascii'),
            base64.b64encode(b"key2").decode('ascii')
        ]
        result = await vault_client.unseal(keys, 2)
        
        assert result == {'sealed': False}
        assert mock_hvac_client.sys.submit_unseal_key.call_count == 2
    
    @pytest.mark.asyncio
    async def test_is_initialized(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.is_initialized.return_value = True
        
        result = await vault_client.is_initialized()
        assert result is True
    
    @pytest.mark.asyncio
    async def test_is_initialized_exception(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.is_initialized.side_effect = Exception("Error")
        
        result = await vault_client.is_initialized()
        assert result is False
    
    @pytest.mark.asyncio
    async def test_health_check(self, vault_client, mock_hvac_client):
        expected_health = {'initialized': True, 'sealed': False}
        mock_hvac_client.sys.read_health_status.return_value = expected_health
        
        result = await vault_client.health_check()
        assert result == expected_health
    
    @pytest.mark.asyncio
    async def test_health_check_exception(self, vault_client, mock_hvac_client):
        mock_hvac_client.sys.read_health_status.side_effect = Exception("Health check failed")
        
        result = await vault_client.health_check()
        assert result == {'initialized': False, 'sealed': True}
    
    def test_security_headers_configured(self, mock_hvac_client):
        with patch('vault_autounseal_operator.vault_client.requests.Session') as mock_session_class:
            mock_session = Mock()
            mock_session_class.return_value = mock_session
            
            VaultClient("https://vault.example.com:8200")
            
            # Verify security headers are set
            mock_session.headers.update.assert_called_once()
            headers = mock_session.headers.update.call_args[0][0]
            
            assert 'User-Agent' in headers
            assert 'X-Content-Type-Options' in headers
            assert 'X-Frame-Options' in headers
            assert headers['X-Content-Type-Options'] == 'nosniff'
            assert headers['X-Frame-Options'] == 'DENY'
    
    def test_retry_strategy_configured(self, mock_hvac_client):
        with patch('vault_autounseal_operator.vault_client.requests.Session') as mock_session_class, \
             patch('vault_autounseal_operator.vault_client.HTTPAdapter') as mock_adapter_class:
            
            mock_session = Mock()
            mock_session_class.return_value = mock_session
            mock_adapter = Mock()
            mock_adapter_class.return_value = mock_adapter
            
            VaultClient("https://vault.example.com:8200")
            
            # Verify adapters are mounted
            assert mock_session.mount.call_count == 2
            mock_session.mount.assert_any_call("http://", mock_adapter)
            mock_session.mount.assert_any_call("https://", mock_adapter)