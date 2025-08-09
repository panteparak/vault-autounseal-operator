import base64
import logging
from typing import Any, Dict, List

import hvac
import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

from .security import SecurityValidator

logger = logging.getLogger(__name__)


class VaultClient:
    def __init__(self, url: str, tls_skip_verify: bool = False, timeout: int = 30):
        # Validate URL for security
        self.url = SecurityValidator.validate_url(url)
        self.timeout = timeout

        # Configure TLS verification
        if tls_skip_verify:
            logger.warning(f"TLS verification disabled for {self.url}")
            verify = False
            # Disable SSL warnings when verification is skipped
            import urllib3

            urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
        else:
            verify = True

        # Configure retry strategy
        retry_strategy = Retry(
            total=3,
            backoff_factor=1,
            status_forcelist=[429, 500, 502, 503, 504],
            allowed_methods=["GET", "POST", "PUT"],
        )

        # Create session with security headers
        session = requests.Session()
        session.mount("http://", HTTPAdapter(max_retries=retry_strategy))
        session.mount("https://", HTTPAdapter(max_retries=retry_strategy))

        # Security headers
        session.headers.update(
            {
                "User-Agent": "vault-autounseal-operator/1.0",
                "X-Content-Type-Options": "nosniff",
                "X-Frame-Options": "DENY",
            }
        )

        self.client = hvac.Client(
            url=self.url, verify=verify, session=session, timeout=self.timeout
        )

    async def is_sealed(self) -> bool:
        try:
            # Run in thread pool to avoid blocking the event loop
            import asyncio

            loop = asyncio.get_event_loop()
            status = await loop.run_in_executor(None, self.client.sys.read_seal_status)
            return status.get("sealed", True)
        except Exception as e:
            logger.error(f"Failed to check seal status for {self.url}: {e}")
            raise

    async def get_seal_status(self) -> Dict[str, Any]:
        try:
            import asyncio

            loop = asyncio.get_event_loop()
            return await loop.run_in_executor(None, self.client.sys.read_seal_status)
        except Exception as e:
            logger.error(f"Failed to get seal status for {self.url}: {e}")
            raise

    async def unseal(self, keys: List[str], threshold: int) -> Dict[str, Any]:
        try:
            # Validate inputs
            if not keys:
                raise ValueError("No unseal keys provided")
            if threshold < 1:
                raise ValueError("Threshold must be at least 1")
            if threshold > len(keys):
                raise ValueError("Threshold exceeds number of available keys")

            # Validate unseal keys format
            validated_keys = SecurityValidator.validate_unseal_keys(keys)

            seal_status = await self.get_seal_status()
            if not seal_status.get("sealed", False):
                logger.info(f"Vault at {self.url} is already unsealed")
                return seal_status

            logger.info(f"Attempting to unseal vault at {self.url}")

            keys_submitted = 0
            last_error = None

            for i, key in enumerate(validated_keys[:threshold]):
                try:
                    # Decode base64 key securely
                    try:
                        decoded_key = base64.b64decode(key, validate=True).decode(
                            "utf-8"
                        )
                    except Exception as e:
                        raise ValueError(f"Invalid base64 encoding in key {i+1}: {e}")

                    # Submit key with timeout in thread pool
                    import asyncio

                    loop = asyncio.get_event_loop()
                    result = await loop.run_in_executor(
                        None, self.client.sys.submit_unseal_key, decoded_key
                    )
                    keys_submitted += 1

                    # Clear the decoded key from memory immediately
                    decoded_key = None

                    if not result.get("sealed", True):
                        logger.info(
                            f"Successfully unsealed vault at {self.url} with {keys_submitted} keys"
                        )
                        return result

                except Exception as e:
                    last_error = e
                    logger.error(
                        f"Failed to submit unseal key {i+1} to {self.url}: {e}"
                    )
                    # Don't break on individual key failures - try remaining keys
                    continue

            final_status = await self.get_seal_status()
            if final_status.get("sealed", True):
                error_msg = f"Vault at {self.url} remains sealed after submitting {keys_submitted} keys"
                if last_error:
                    error_msg += f". Last error: {last_error}"
                logger.warning(error_msg)
            else:
                logger.info(f"Successfully unsealed vault at {self.url}")

            return final_status

        except Exception as e:
            # Don't log the actual keys for security
            sanitized_error = (
                str(e).replace(str(keys), "[REDACTED]") if keys else str(e)
            )
            logger.error(f"Failed to unseal vault at {self.url}: {sanitized_error}")
            raise

    async def is_initialized(self) -> bool:
        try:
            import asyncio

            loop = asyncio.get_event_loop()
            status = await loop.run_in_executor(None, self.client.sys.is_initialized)
            return status
        except Exception as e:
            logger.error(f"Failed to check initialization status for {self.url}: {e}")
            return False

    async def health_check(self) -> Dict[str, Any]:
        try:
            import asyncio

            loop = asyncio.get_event_loop()
            health = await loop.run_in_executor(
                None, self.client.sys.read_health_status
            )
            return health
        except Exception as e:
            logger.error(f"Health check failed for {self.url}: {e}")
            return {"initialized": False, "sealed": True}
