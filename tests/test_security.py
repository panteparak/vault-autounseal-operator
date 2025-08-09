import pytest
from vault_autounseal_operator.security import SecurityValidator


class TestSecurityValidator:
    
    def test_validate_url_valid(self):
        # Valid URLs
        assert SecurityValidator.validate_url("https://vault.example.com:8200") == "https://vault.example.com:8200"
        assert SecurityValidator.validate_url("http://localhost:8200") == "http://localhost:8200"
        assert SecurityValidator.validate_url("https://vault.example.com/v1") == "https://vault.example.com/v1"
    
    def test_validate_url_invalid(self):
        # Empty URL
        with pytest.raises(ValueError, match="URL cannot be empty"):
            SecurityValidator.validate_url("")
        
        # No scheme
        with pytest.raises(ValueError, match="URL must include scheme"):
            SecurityValidator.validate_url("vault.example.com")
        
        # Invalid scheme
        with pytest.raises(ValueError, match="URL scheme must be one of"):
            SecurityValidator.validate_url("ftp://vault.example.com")
        
        # No hostname
        with pytest.raises(ValueError, match="URL must include hostname"):
            SecurityValidator.validate_url("https://")
        
        # Too long URL
        long_url = "https://" + "a" * 2100
        with pytest.raises(ValueError, match="URL exceeds maximum length"):
            SecurityValidator.validate_url(long_url)
    
    def test_validate_kubernetes_name_valid(self):
        assert SecurityValidator.validate_kubernetes_name("valid-name") == "valid-name"
        assert SecurityValidator.validate_kubernetes_name("test123") == "test123"
        assert SecurityValidator.validate_kubernetes_name("a") == "a"
    
    def test_validate_kubernetes_name_invalid(self):
        # Empty name
        with pytest.raises(ValueError, match="name cannot be empty"):
            SecurityValidator.validate_kubernetes_name("")
        
        # Invalid characters
        with pytest.raises(ValueError, match="must be a valid DNS label"):
            SecurityValidator.validate_kubernetes_name("INVALID")
        
        with pytest.raises(ValueError, match="must be a valid DNS label"):
            SecurityValidator.validate_kubernetes_name("invalid_name")
        
        # Too long
        long_name = "a" * 300
        with pytest.raises(ValueError, match="exceeds maximum length"):
            SecurityValidator.validate_kubernetes_name(long_name)
    
    def test_validate_secret_key_valid(self):
        assert SecurityValidator.validate_secret_key("valid-key") == "valid-key"
        assert SecurityValidator.validate_secret_key("key.name") == "key.name"
        assert SecurityValidator.validate_secret_key("key_name") == "key_name"
    
    def test_validate_secret_key_invalid(self):
        # Empty key
        with pytest.raises(ValueError, match="Secret key cannot be empty"):
            SecurityValidator.validate_secret_key("")
        
        # Invalid characters
        with pytest.raises(ValueError, match="contains invalid characters"):
            SecurityValidator.validate_secret_key("key with spaces")
        
        # Too long
        long_key = "a" * 300
        with pytest.raises(ValueError, match="exceeds maximum length"):
            SecurityValidator.validate_secret_key(long_key)
    
    def test_validate_unseal_keys_valid(self):
        import base64
        
        # Create valid base64 keys
        key1 = base64.b64encode(b"test-key-1").decode('ascii')
        key2 = base64.b64encode(b"test-key-2").decode('ascii')
        keys = [key1, key2]
        
        result = SecurityValidator.validate_unseal_keys(keys)
        assert result == keys
    
    def test_validate_unseal_keys_invalid(self):
        # Empty keys
        with pytest.raises(ValueError, match="Unseal keys cannot be empty"):
            SecurityValidator.validate_unseal_keys([])
        
        # Too many keys
        keys = ["a"] * 15
        with pytest.raises(ValueError, match="Too many unseal keys"):
            SecurityValidator.validate_unseal_keys(keys)
        
        # Empty key in list
        with pytest.raises(ValueError, match="Unseal key 1 cannot be empty"):
            SecurityValidator.validate_unseal_keys([""])
        
        # Invalid base64
        with pytest.raises(ValueError, match="is not valid base64"):
            SecurityValidator.validate_unseal_keys(["invalid-base64!"])
        
        # Base64 that decodes to empty
        import base64
        empty_key = base64.b64encode(b"").decode('ascii')
        with pytest.raises(ValueError, match="decodes to empty string"):
            SecurityValidator.validate_unseal_keys([empty_key])
    
    def test_validate_threshold_valid(self):
        assert SecurityValidator.validate_threshold(3, 5) == 3
        assert SecurityValidator.validate_threshold(1, 1) == 1
    
    def test_validate_threshold_invalid(self):
        # Too low
        with pytest.raises(ValueError, match="Threshold must be at least 1"):
            SecurityValidator.validate_threshold(0, 5)
        
        # Exceeds number of keys
        with pytest.raises(ValueError, match="cannot exceed number of keys"):
            SecurityValidator.validate_threshold(5, 3)
        
        # Exceeds maximum
        with pytest.raises(ValueError, match="exceeds maximum"):
            SecurityValidator.validate_threshold(15, 20)
    
    def test_sanitize_log_data(self):
        # Test dictionary with sensitive keys
        data = {
            "url": "https://vault.example.com",
            "unseal_key": "secret-value",
            "password": "secret-password",
            "metadata": {
                "token": "secret-token",
                "name": "public-name"
            }
        }
        
        sanitized = SecurityValidator.sanitize_log_data(data)
        
        assert sanitized["url"] == "https://vault.example.com"
        assert sanitized["unseal_key"] == "[REDACTED]"
        assert sanitized["password"] == "[REDACTED]"
        assert sanitized["metadata"]["token"] == "[REDACTED]"
        assert sanitized["metadata"]["name"] == "public-name"
    
    def test_validate_spec_valid_direct_keys(self):
        import base64
        
        key1 = base64.b64encode(b"test-key-1").decode('ascii')
        key2 = base64.b64encode(b"test-key-2").decode('ascii')
        
        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {
                "secret": [key1, key2]
            },
            "threshold": 2
        }
        
        result = SecurityValidator.validate_spec(spec)
        
        assert result["url"] == "https://vault.example.com:8200"
        assert result["unsealKeys"]["secret"] == [key1, key2]
        assert result["threshold"] == 2
        assert result["namespace"] == "default"  # Default value
        assert result["haEnabled"] is False  # Default value
    
    def test_validate_spec_valid_secret_ref(self):
        spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {
                "secretRef": {
                    "name": "vault-keys",
                    "namespace": "vault",
                    "key": "keys"
                }
            }
        }
        
        result = SecurityValidator.validate_spec(spec)
        
        assert result["url"] == "https://vault.example.com:8200"
        assert result["unsealKeys"]["secretRef"]["name"] == "vault-keys"
        assert result["unsealKeys"]["secretRef"]["namespace"] == "vault"
        assert result["unsealKeys"]["secretRef"]["key"] == "keys"
    
    def test_validate_spec_invalid(self):
        # Missing URL
        with pytest.raises(ValueError, match="URL is required"):
            SecurityValidator.validate_spec({"unsealKeys": {"secret": ["key"]}})
        
        # Missing unsealKeys
        with pytest.raises(ValueError, match="unsealKeys is required"):
            SecurityValidator.validate_spec({"url": "https://vault.example.com"})
        
        # Both secret and secretRef specified
        spec = {
            "url": "https://vault.example.com",
            "unsealKeys": {
                "secret": ["key"],
                "secretRef": {"name": "secret"}
            }
        }
        with pytest.raises(ValueError, match="Cannot specify both"):
            SecurityValidator.validate_spec(spec)
        
        # Neither secret nor secretRef specified
        spec = {
            "url": "https://vault.example.com",
            "unsealKeys": {}
        }
        with pytest.raises(ValueError, match="Must specify either"):
            SecurityValidator.validate_spec(spec)
        
        # Invalid reconcileInterval
        spec = {
            "url": "https://vault.example.com",
            "unsealKeys": {"secret": ["dGVzdA=="]},
            "reconcileInterval": "invalid"
        }
        with pytest.raises(ValueError, match="reconcileInterval must be in format"):
            SecurityValidator.validate_spec(spec)