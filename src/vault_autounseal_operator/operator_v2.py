import asyncio
import logging
import kopf
from typing import Dict, Any, Optional, List
from datetime import datetime, timezone
from kubernetes import client
from kubernetes.client.rest import ApiException
import base64
import json

from .vault_client import VaultClient
from .pod_watcher import PodWatcher
from .models import VaultUnsealConfigSpec, VaultUnsealConfigStatus, VaultStatus, Condition
from .security import SecurityValidator

logger = logging.getLogger(__name__)


class VaultOperatorV2:
    def __init__(self):
        self.vault_clients: Dict[str, VaultClient] = {}
        self.pod_watchers: Dict[str, PodWatcher] = {}
        self.k8s_client = client.CoreV1Api()
    
    async def get_unseal_keys(self, spec: Dict[str, Any], namespace: str) -> List[str]:
        """Extract unseal keys from either direct secret or secretRef"""
        unseal_keys_spec = spec.get('unsealKeys', {})
        
        # Direct secret keys
        if 'secret' in unseal_keys_spec and unseal_keys_spec['secret']:
            keys = unseal_keys_spec['secret']
            if not isinstance(keys, list):
                raise ValueError("unsealKeys.secret must be a list")
            return keys
        
        # Secret reference
        if 'secretRef' in unseal_keys_spec and unseal_keys_spec['secretRef']:
            secret_ref = unseal_keys_spec['secretRef']
            if not isinstance(secret_ref, dict) or 'name' not in secret_ref:
                raise ValueError("unsealKeys.secretRef must be an object with 'name' field")
                
            secret_name = secret_ref['name']
            secret_namespace = secret_ref.get('namespace', namespace)
            key = secret_ref.get('key', 'unseal-keys')
            
            try:
                secret = self.k8s_client.read_namespaced_secret(
                    name=secret_name,
                    namespace=secret_namespace
                )
                
                if key in secret.data:
                    # Decode base64 encoded secret data
                    try:
                        encoded_keys = base64.b64decode(secret.data[key]).decode('utf-8')
                    except Exception as e:
                        raise ValueError(f"Failed to decode secret data: {e}")
                        
                    # Parse as JSON array or newline-separated
                    try:
                        parsed_keys = json.loads(encoded_keys)
                        if not isinstance(parsed_keys, list):
                            raise ValueError("Secret data must contain a JSON array")
                        return parsed_keys
                    except json.JSONDecodeError:
                        # Try newline-separated format
                        keys = [k.strip() for k in encoded_keys.split('\n') if k.strip()]
                        if not keys:
                            raise ValueError("No valid keys found in secret")
                        return keys
                else:
                    raise ValueError(f"Key '{key}' not found in secret '{secret_name}'")
                    
            except ApiException as e:
                raise ValueError(f"Failed to read secret '{secret_name}': {e}")
        
        raise ValueError("No unseal keys provided in either 'secret' or 'secretRef'")
    
    async def setup_vault_client(self, spec: Dict[str, Any], namespace: str, name: str):
        """Setup vault client and optionally pod watcher"""
        url = spec['url']
        tls_skip_verify = spec.get('tlsSkipVerify', False)
        
        client_key = f"{namespace}/{name}"
        self.vault_clients[client_key] = VaultClient(url, tls_skip_verify)
        
        # Setup pod watcher if HA is enabled
        if spec.get('haEnabled', False):
            pod_selector = spec.get('podSelector', {})
            target_namespace = spec.get('namespace', namespace)
            unseal_keys = await self.get_unseal_keys(spec, namespace)
            threshold = spec.get('threshold', 3)
            
            watcher = PodWatcher(
                namespace=target_namespace,
                pod_selector=pod_selector,
                vault_client=self.vault_clients[client_key],
                unseal_keys=unseal_keys,
                threshold=threshold
            )
            
            self.pod_watchers[client_key] = watcher
            await watcher.start()
    
    async def cleanup_vault_instance(self, namespace: str, name: str):
        """Cleanup vault client and pod watcher"""
        client_key = f"{namespace}/{name}"
        
        if client_key in self.pod_watchers:
            await self.pod_watchers[client_key].stop()
            del self.pod_watchers[client_key]
        
        if client_key in self.vault_clients:
            del self.vault_clients[client_key]
    
    async def check_and_unseal_vault(self, spec: Dict[str, Any], namespace: str, name: str) -> VaultStatus:
        """Check vault status and unseal if needed"""
        client_key = f"{namespace}/{name}"
        
        if client_key not in self.vault_clients:
            await self.setup_vault_client(spec, namespace, name)
        
        vault_client = self.vault_clients[client_key]
        
        try:
            is_sealed = await vault_client.is_sealed()
            
            if is_sealed:
                logger.info(f"Vault {name} is sealed, attempting to unseal...")
                unseal_keys = await self.get_unseal_keys(spec, namespace)
                threshold = spec.get('threshold', 3)
                
                result = await vault_client.unseal(unseal_keys, threshold)
                is_sealed = result.get('sealed', True)
            
            return VaultStatus(
                sealed=is_sealed,
                lastUnsealed=datetime.now(timezone.utc).isoformat() if not is_sealed else None,
                lastChecked=datetime.now(timezone.utc).isoformat(),
                error=None
            )
        
        except Exception as e:
            logger.error(f"Failed to check/unseal vault {name}: {e}")
            return VaultStatus(
                sealed=True,
                lastUnsealed=None,
                lastChecked=datetime.now(timezone.utc).isoformat(),
                error=str(e)
            )


operator = VaultOperatorV2()


@kopf.on.create('vault.io', 'v1', 'vaultunsealconfigs')
@kopf.on.resume('vault.io', 'v1', 'vaultunsealconfigs')
async def create_vault_config(body, name, namespace, **kwargs):
    """Handle creation and resumption of VaultUnsealConfig"""
    logger.info(f"Creating/resuming VaultUnsealConfig {name} in namespace {namespace}")
    
    spec = body.get('spec', {})
    
    try:
        # Validate and sanitize spec for security
        validated_spec = SecurityValidator.validate_spec(spec)
        logger.debug(f"Validated spec: {SecurityValidator.sanitize_log_data(validated_spec)}")
        
        await operator.setup_vault_client(validated_spec, namespace, name)
        vault_status = await operator.check_and_unseal_vault(validated_spec, namespace, name)
        
        return {
            'conditions': [{
                'type': 'Ready',
                'status': 'True' if not vault_status.error else 'False',
                'lastTransitionTime': datetime.now(timezone.utc).isoformat(),
                'reason': 'ConfigurationApplied' if not vault_status.error else 'Error',
                'message': f'Vault configuration applied successfully' if not vault_status.error else vault_status.error
            }],
            'vaultStatus': {
                'sealed': vault_status.sealed,
                'lastUnsealed': vault_status.lastUnsealed,
                'lastChecked': vault_status.lastChecked,
                'error': vault_status.error
            }
        }
    
    except Exception as e:
        logger.error(f"Failed to create VaultUnsealConfig {name}: {e}")
        return {
            'conditions': [{
                'type': 'Ready',
                'status': 'False',
                'lastTransitionTime': datetime.now(timezone.utc).isoformat(),
                'reason': 'Error',
                'message': str(e)
            }]
        }


@kopf.on.update('vault.io', 'v1', 'vaultunsealconfigs')
async def update_vault_config(body, name, namespace, old, new, **kwargs):
    """Handle updates to VaultUnsealConfig"""
    logger.info(f"Updating VaultUnsealConfig {name} in namespace {namespace}")
    
    try:
        # Cleanup old configuration
        await operator.cleanup_vault_instance(namespace, name)
        
        # Apply new configuration
        return await create_vault_config(body, name, namespace, **kwargs)
    except Exception as e:
        logger.error(f"Failed to update VaultUnsealConfig {name}: {e}")
        return {
            'conditions': [{
                'type': 'Ready',
                'status': 'False',
                'lastTransitionTime': datetime.now(timezone.utc).isoformat(),
                'reason': 'UpdateError',
                'message': str(e)
            }]
        }


@kopf.on.delete('vault.io', 'v1', 'vaultunsealconfigs')
async def delete_vault_config(body, name, namespace, **kwargs):
    """Handle deletion of VaultUnsealConfig"""
    logger.info(f"Deleting VaultUnsealConfig {name} in namespace {namespace}")
    
    await operator.cleanup_vault_instance(namespace, name)


@kopf.timer('vault.io', 'v1', 'vaultunsealconfigs', interval=30)
async def reconcile_vault_config(body, name, namespace, **kwargs):
    """Periodic reconciliation of vault status"""
    logger.debug(f"Reconciling VaultUnsealConfig {name} in namespace {namespace}")
    
    spec = body.get('spec', {})
    
    try:
        vault_status = await operator.check_and_unseal_vault(spec, namespace, name)
        
        return {
            'vaultStatus': {
                'sealed': vault_status.sealed,
                'lastUnsealed': vault_status.lastUnsealed,
                'lastChecked': vault_status.lastChecked,
                'error': vault_status.error
            }
        }
    
    except Exception as e:
        logger.error(f"Failed to reconcile VaultUnsealConfig {name}: {e}")
        return {
            'vaultStatus': {
                'sealed': True,
                'lastUnsealed': None,
                'lastChecked': datetime.now(timezone.utc).isoformat(),
                'error': str(e)
            }
        }