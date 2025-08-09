import asyncio
import logging
from typing import Any, Dict, List, Optional

from kubernetes import client, watch
from kubernetes.client.rest import ApiException

from .vault_client import VaultClient

logger = logging.getLogger(__name__)


class PodWatcher:
    def __init__(
        self,
        namespace: str,
        pod_selector: Dict[str, Any],
        vault_client: VaultClient,
        unseal_keys: List[str],
        threshold: int = 3,
    ):
        self.namespace = namespace
        self.pod_selector = pod_selector
        self.vault_client = vault_client
        self.unseal_keys = unseal_keys
        self.threshold = threshold
        self.v1 = client.CoreV1Api()
        self.watch_task: Optional[asyncio.Task] = None
        self.running = False
        self.monitored_pods: Dict[str, Dict] = {}

    async def start(self):
        if self.running:
            return

        self.running = True
        self.watch_task = asyncio.create_task(self._watch_pods())
        logger.info(f"Started pod watcher for namespace {self.namespace}")

    async def stop(self):
        self.running = False
        if self.watch_task:
            self.watch_task.cancel()
            try:
                await self.watch_task
            except asyncio.CancelledError:
                pass
        logger.info(f"Stopped pod watcher for namespace {self.namespace}")

    def _matches_selector(self, pod_labels: Dict[str, str]) -> bool:
        match_labels = self.pod_selector.get("matchLabels", {})

        for key, value in match_labels.items():
            if key not in pod_labels or pod_labels[key] != value:
                return False

        return True

    async def _is_vault_pod_ready_and_sealed(self, pod: Dict[str, Any]) -> bool:
        try:
            if pod["status"]["phase"] != "Running":
                return False

            pod_ip = pod["status"].get("podIP")
            if not pod_ip:
                return False

            container_statuses = pod["status"].get("containerStatuses", [])
            all_ready = all(status.get("ready", False) for status in container_statuses)

            if not all_ready:
                return False

            is_sealed = await self.vault_client.is_sealed()
            return is_sealed

        except Exception as e:
            logger.error(f"Error checking pod readiness and vault seal status: {e}")
            return False

    async def _handle_pod_event(self, event_type: str, pod: Dict[str, Any]):
        pod_name = pod["metadata"]["name"]
        pod_uid = pod["metadata"]["uid"]

        if not self._matches_selector(pod["metadata"].get("labels", {})):
            return

        logger.debug(f"Processing {event_type} event for pod {pod_name}")

        if event_type == "DELETED":
            if pod_uid in self.monitored_pods:
                del self.monitored_pods[pod_uid]
                logger.info(f"Removed pod {pod_name} from monitoring")
            return

        if event_type in ["ADDED", "MODIFIED"]:
            if await self._is_vault_pod_ready_and_sealed(pod):
                if pod_uid not in self.monitored_pods:
                    logger.info(f"Detected new sealed vault pod: {pod_name}")

                self.monitored_pods[pod_uid] = {
                    "name": pod_name,
                    "namespace": self.namespace,
                    "last_unseal_attempt": 0,
                }

                await self._attempt_unseal(pod_name)
            elif pod_uid in self.monitored_pods:
                del self.monitored_pods[pod_uid]
                logger.debug(
                    f"Pod {pod_name} no longer sealed or not ready, removed from monitoring"
                )

    async def _attempt_unseal(self, pod_name: str):
        try:
            logger.info(f"Attempting to unseal vault in pod {pod_name}")
            result = await self.vault_client.unseal(self.unseal_keys, self.threshold)

            if not result.get("sealed", True):
                logger.info(f"Successfully unsealed vault in pod {pod_name}")
            else:
                logger.warning(
                    f"Vault in pod {pod_name} remains sealed after unseal attempt"
                )

        except Exception as e:
            logger.error(f"Failed to unseal vault in pod {pod_name}: {e}")

    async def _watch_pods(self):
        w = watch.Watch()

        while self.running:
            try:
                logger.info(f"Starting pod watch stream for namespace {self.namespace}")

                stream = w.stream(
                    self.v1.list_namespaced_pod,
                    namespace=self.namespace,
                    timeout_seconds=60,
                )

                async for event in self._async_stream(stream):
                    if not self.running:
                        break

                    event_type = event["type"]
                    pod = event["object"]

                    await self._handle_pod_event(event_type, pod)

            except ApiException as e:
                if e.status == 410:
                    logger.info("Pod watch stream expired, restarting...")
                    continue
                else:
                    logger.error(f"API error during pod watching: {e}")
                    await asyncio.sleep(5)

            except Exception as e:
                logger.error(f"Unexpected error during pod watching: {e}")
                await asyncio.sleep(5)

        w.stop()

    async def _async_stream(self, stream):
        loop = asyncio.get_event_loop()

        def stream_generator():
            try:
                for event in stream:
                    yield event
            except Exception as e:
                logger.error(f"Stream generator error: {e}")
                return

        gen = stream_generator()

        while self.running:
            try:
                event = await loop.run_in_executor(None, next, gen)
                yield event
            except StopIteration:
                break
            except Exception as e:
                logger.error(f"Error in async stream: {e}")
                break

    async def get_monitored_pods(self) -> List[Dict[str, Any]]:
        return list(self.monitored_pods.values())
