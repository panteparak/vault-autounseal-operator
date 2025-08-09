import re
import logging
from typing import List, Dict, Any, Optional
import base64
import json
from urllib.parse import urlparse

logger = logging.getLogger(__name__)


class SecurityValidator:
    """Security validation utilities for the operator"""
    
    # Allowed URL schemes
    ALLOWED_SCHEMES = {'https', 'http'}
    
    # Maximum length for various fields to prevent DoS
    MAX_URL_LENGTH = 2048
    MAX_SECRET_NAME_LENGTH = 253
    MAX_NAMESPACE_LENGTH = 63
    MAX_KEY_LENGTH = 253
    MAX_UNSEAL_KEYS = 10
    MAX_UNSEAL_KEY_LENGTH = 1024
    
    # Regex patterns for validation
    DNS_LABEL_REGEX = re.compile(r'^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')
    SECRET_KEY_REGEX = re.compile(r'^[a-zA-Z0-9._-]+$')
    
    @classmethod
    def validate_url(cls, url: str) -> str:
        """Validate and sanitize Vault URL"""
        if not url:
            raise ValueError("URL cannot be empty")
            
        if len(url) > cls.MAX_URL_LENGTH:
            raise ValueError(f"URL exceeds maximum length of {cls.MAX_URL_LENGTH}")
            
        try:
            parsed = urlparse(url)
            
            if not parsed.scheme:
                raise ValueError("URL must include scheme (http/https)")
                
            if parsed.scheme.lower() not in cls.ALLOWED_SCHEMES:
                raise ValueError(f"URL scheme must be one of: {', '.join(cls.ALLOWED_SCHEMES)}")
                
            if not parsed.netloc:
                raise ValueError("URL must include hostname")
                
            # Prevent localhost/private IPs in production (optional)
            hostname = parsed.hostname
            if hostname:
                if hostname.lower() in ['localhost', '127.0.0.1', '::1']:
                    logger.warning(f"Using localhost URL: {url}")
                    
            # Reconstruct clean URL
            clean_url = f"{parsed.scheme}://{parsed.netloc}"
            if parsed.path and parsed.path != '/':
                clean_url += parsed.path
                
            return clean_url
            
        except Exception as e:
            raise ValueError(f"Invalid URL format: {e}")
    
    @classmethod
    def validate_kubernetes_name(cls, name: str, field_name: str = "name") -> str:
        """Validate Kubernetes resource names"""
        if not name:
            raise ValueError(f"{field_name} cannot be empty")
            
        max_length = cls.MAX_SECRET_NAME_LENGTH if field_name == "secret name" else cls.MAX_NAMESPACE_LENGTH
        
        if len(name) > max_length:
            raise ValueError(f"{field_name} exceeds maximum length of {max_length}")
            
        if not cls.DNS_LABEL_REGEX.match(name.lower()):
            raise ValueError(f"{field_name} must be a valid DNS label")
            
        return name.lower()
    
    @classmethod
    def validate_secret_key(cls, key: str) -> str:
        """Validate secret key names"""
        if not key:
            raise ValueError("Secret key cannot be empty")
            
        if len(key) > cls.MAX_KEY_LENGTH:
            raise ValueError(f"Secret key exceeds maximum length of {cls.MAX_KEY_LENGTH}")
            
        if not cls.SECRET_KEY_REGEX.match(key):
            raise ValueError("Secret key contains invalid characters")
            
        return key
    
    @classmethod
    def validate_unseal_keys(cls, keys: List[str]) -> List[str]:
        """Validate direct unseal keys"""
        if not keys:
            raise ValueError("Unseal keys cannot be empty")
            
        if len(keys) > cls.MAX_UNSEAL_KEYS:
            raise ValueError(f"Too many unseal keys (max: {cls.MAX_UNSEAL_KEYS})")
            
        validated_keys = []
        for i, key in enumerate(keys):
            if not key:
                raise ValueError(f"Unseal key {i+1} cannot be empty")
                
            if len(key) > cls.MAX_UNSEAL_KEY_LENGTH:
                raise ValueError(f"Unseal key {i+1} exceeds maximum length")
                
            # Validate base64 encoding
            try:
                decoded = base64.b64decode(key)
                if len(decoded) == 0:
                    raise ValueError(f"Unseal key {i+1} decodes to empty string")
            except Exception as e:
                raise ValueError(f"Unseal key {i+1} is not valid base64: {e}")
                
            validated_keys.append(key)
            
        return validated_keys
    
    @classmethod
    def validate_threshold(cls, threshold: int, num_keys: int) -> int:
        """Validate unseal threshold"""
        if threshold < 1:
            raise ValueError("Threshold must be at least 1")
            
        if threshold > num_keys:
            raise ValueError(f"Threshold ({threshold}) cannot exceed number of keys ({num_keys})")
            
        if threshold > cls.MAX_UNSEAL_KEYS:
            raise ValueError(f"Threshold exceeds maximum ({cls.MAX_UNSEAL_KEYS})")
            
        return threshold
    
    @classmethod
    def sanitize_log_data(cls, data: Any) -> Any:
        """Sanitize data for logging (remove sensitive information)"""
        if isinstance(data, dict):
            sanitized = {}
            for key, value in data.items():
                if any(sensitive in key.lower() for sensitive in ['key', 'secret', 'token', 'password']):
                    sanitized[key] = "[REDACTED]"
                else:
                    sanitized[key] = cls.sanitize_log_data(value)
            return sanitized
        elif isinstance(data, list):
            return [cls.sanitize_log_data(item) for item in data]
        else:
            return data
    
    @classmethod
    def validate_spec(cls, spec: Dict[str, Any]) -> Dict[str, Any]:
        """Validate complete VaultUnsealConfig spec"""
        validated_spec = {}
        
        # Validate URL
        if 'url' not in spec:
            raise ValueError("URL is required")
        validated_spec['url'] = cls.validate_url(spec['url'])
        
        # Validate unseal keys
        if 'unsealKeys' not in spec:
            raise ValueError("unsealKeys is required")
            
        unseal_keys = spec['unsealKeys']
        if 'secret' in unseal_keys and 'secretRef' in unseal_keys:
            raise ValueError("Cannot specify both 'secret' and 'secretRef' in unsealKeys")
            
        if 'secret' not in unseal_keys and 'secretRef' not in unseal_keys:
            raise ValueError("Must specify either 'secret' or 'secretRef' in unsealKeys")
            
        if 'secret' in unseal_keys:
            validated_spec['unsealKeys'] = {
                'secret': cls.validate_unseal_keys(unseal_keys['secret'])
            }
            num_keys = len(unseal_keys['secret'])
        else:
            secret_ref = unseal_keys['secretRef']
            if 'name' not in secret_ref:
                raise ValueError("secretRef.name is required")
                
            validated_secret_ref = {
                'name': cls.validate_kubernetes_name(secret_ref['name'], "secret name"),
                'key': cls.validate_secret_key(secret_ref.get('key', 'unseal-keys'))
            }
            
            if 'namespace' in secret_ref:
                validated_secret_ref['namespace'] = cls.validate_kubernetes_name(
                    secret_ref['namespace'], "namespace"
                )
                
            validated_spec['unsealKeys'] = {'secretRef': validated_secret_ref}
            num_keys = cls.MAX_UNSEAL_KEYS  # Assume max for validation
        
        # Validate optional fields
        if 'namespace' in spec:
            validated_spec['namespace'] = cls.validate_kubernetes_name(
                spec['namespace'], "namespace"
            )
        else:
            validated_spec['namespace'] = 'default'
            
        if 'threshold' in spec:
            validated_spec['threshold'] = cls.validate_threshold(
                spec['threshold'], num_keys
            )
        else:
            validated_spec['threshold'] = min(3, num_keys)
            
        # Boolean fields
        validated_spec['haEnabled'] = bool(spec.get('haEnabled', False))
        validated_spec['tlsSkipVerify'] = bool(spec.get('tlsSkipVerify', False))
        
        # Validate interval
        interval = spec.get('reconcileInterval', '30s')
        if not re.match(r'^\d+[smh]$', interval):
            raise ValueError("reconcileInterval must be in format like '30s', '5m', '1h'")
        validated_spec['reconcileInterval'] = interval
        
        # Validate podSelector if present
        if 'podSelector' in spec and spec['podSelector']:
            pod_selector = spec['podSelector']
            if 'matchLabels' in pod_selector:
                for key, value in pod_selector['matchLabels'].items():
                    if not isinstance(key, str) or not isinstance(value, str):
                        raise ValueError("podSelector.matchLabels keys and values must be strings")
                    if len(key) > 253 or len(value) > 63:
                        raise ValueError("podSelector label key/value too long")
                validated_spec['podSelector'] = pod_selector
        
        return validated_spec