import logging
from datetime import datetime, timezone
from typing import Any, Dict

import kopf

from .crd_manager import CRDManager
from .pod_watcher import PodWatcher
from .vault_client import VaultClient

logger = logging.getLogger(__name__)


class VaultUnsealOperator:
    def __init__(self):
        self.pod_watchers: Dict[str, PodWatcher] = {}
        self.vault_clients: Dict[str, VaultClient] = {}
        self.crd_manager = CRDManager()

    async def ensure_crd_exists(self):
        await self.crd_manager.ensure_crd_exists()

    async def setup_vault_instance(
        self, instance_config: Dict[str, Any], namespace: str
    ):
        instance_name = instance_config["name"]
        endpoint = instance_config["endpoint"]
        tls_skip_verify = instance_config.get("tlsSkipVerify", False)

        client_key = f"{namespace}/{instance_name}"
        self.vault_clients[client_key] = VaultClient(endpoint, tls_skip_verify)

        if instance_config.get("haEnabled", False):
            pod_selector = instance_config.get("podSelector", {})
            target_namespace = instance_config.get("namespace", namespace)

            watcher = PodWatcher(
                namespace=target_namespace,
                pod_selector=pod_selector,
                vault_client=self.vault_clients[client_key],
                unseal_keys=instance_config["unsealKeys"],
                threshold=instance_config.get("threshold", 3),
            )

            self.pod_watchers[client_key] = watcher
            await watcher.start()

    async def cleanup_vault_instance(self, instance_name: str, namespace: str):
        client_key = f"{namespace}/{instance_name}"

        if client_key in self.pod_watchers:
            await self.pod_watchers[client_key].stop()
            del self.pod_watchers[client_key]

        if client_key in self.vault_clients:
            del self.vault_clients[client_key]

    async def unseal_vault_instance(
        self, instance_config: Dict[str, Any], namespace: str
    ) -> Dict[str, Any]:
        instance_name = instance_config["name"]
        client_key = f"{namespace}/{instance_name}"

        if client_key not in self.vault_clients:
            await self.setup_vault_instance(instance_config, namespace)

        vault_client = self.vault_clients[client_key]
        unseal_keys = instance_config["unsealKeys"]
        threshold = instance_config.get("threshold", 3)

        try:
            if await vault_client.is_sealed():
                result = await vault_client.unseal(unseal_keys, threshold)
                return {
                    "name": instance_name,
                    "sealed": result.get("sealed", True),
                    "lastUnsealed": datetime.now(timezone.utc).isoformat(),
                    "error": None,
                }
            else:
                return {
                    "name": instance_name,
                    "sealed": False,
                    "lastUnsealed": datetime.now(timezone.utc).isoformat(),
                    "error": None,
                }
        except Exception as e:
            logger.error(f"Failed to unseal vault instance {instance_name}: {e}")
            return {
                "name": instance_name,
                "sealed": True,
                "lastUnsealed": None,
                "error": str(e),
            }


operator_instance = VaultUnsealOperator()


@kopf.on.create("vault.io", "v1", "vaultunsealconfigs")
@kopf.on.resume("vault.io", "v1", "vaultunsealconfigs")
async def create_vault_unseal_config(body, name, namespace, **kwargs):
    logger.info(f"Creating VaultUnsealConfig {name} in namespace {namespace}")

    spec = body.get("spec", {})
    vault_instances = spec.get("vaultInstances", [])

    vault_statuses = []

    for instance in vault_instances:
        await operator_instance.setup_vault_instance(instance, namespace)

        status = await operator_instance.unseal_vault_instance(instance, namespace)
        vault_statuses.append(status)

    return {
        "conditions": [
            {
                "type": "Ready",
                "status": "True",
                "lastTransitionTime": datetime.now(timezone.utc).isoformat(),
                "reason": "ConfigurationApplied",
                "message": f"Configured {len(vault_instances)} vault instances",
            }
        ],
        "vaultStatuses": vault_statuses,
    }


@kopf.on.update("vault.io", "v1", "vaultunsealconfigs")
async def update_vault_unseal_config(body, name, namespace, old, new, **kwargs):
    logger.info(f"Updating VaultUnsealConfig {name} in namespace {namespace}")

    old_spec = old.get("spec", {})
    new_spec = new.get("spec", {})

    old_instances = {inst["name"]: inst for inst in old_spec.get("vaultInstances", [])}
    new_instances = {inst["name"]: inst for inst in new_spec.get("vaultInstances", [])}

    for instance_name in old_instances:
        if instance_name not in new_instances:
            await operator_instance.cleanup_vault_instance(instance_name, namespace)

    vault_statuses = []
    for instance in new_instances.values():
        await operator_instance.setup_vault_instance(instance, namespace)
        status = await operator_instance.unseal_vault_instance(instance, namespace)
        vault_statuses.append(status)

    return {
        "conditions": [
            {
                "type": "Ready",
                "status": "True",
                "lastTransitionTime": datetime.now(timezone.utc).isoformat(),
                "reason": "ConfigurationUpdated",
                "message": f"Updated configuration for {len(new_instances)} vault instances",
            }
        ],
        "vaultStatuses": vault_statuses,
    }


@kopf.on.delete("vault.io", "v1", "vaultunsealconfigs")
async def delete_vault_unseal_config(body, name, namespace, **kwargs):
    logger.info(f"Deleting VaultUnsealConfig {name} in namespace {namespace}")

    spec = body.get("spec", {})
    vault_instances = spec.get("vaultInstances", [])

    for instance in vault_instances:
        await operator_instance.cleanup_vault_instance(instance["name"], namespace)


@kopf.timer("vault.io", "v1", "vaultunsealconfigs", interval=30)
async def reconcile_vault_unseal_config(body, name, namespace, **kwargs):
    logger.debug(f"Reconciling VaultUnsealConfig {name} in namespace {namespace}")

    spec = body.get("spec", {})
    vault_instances = spec.get("vaultInstances", [])

    vault_statuses = []

    for instance in vault_instances:
        status = await operator_instance.unseal_vault_instance(instance, namespace)
        vault_statuses.append(status)

    return {"vaultStatuses": vault_statuses}
