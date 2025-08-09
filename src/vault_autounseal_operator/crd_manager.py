import asyncio
import logging

from kubernetes import client
from kubernetes.client.rest import ApiException

from .crd_schema import CRDGenerator

logger = logging.getLogger(__name__)


class CRDManager:
    def __init__(self):
        self.api_client = client.ApiextensionsV1Api()
        self.crd_name = f"{CRDGenerator.PLURAL}.{CRDGenerator.GROUP}"

    async def ensure_crd_exists(self) -> bool:
        try:
            existing_crd = self.api_client.read_custom_resource_definition(
                name=self.crd_name
            )
            logger.info(f"CRD {self.crd_name} already exists")
            return True

        except ApiException as e:
            if e.status == 404:
                logger.info(f"CRD {self.crd_name} not found, creating...")
                return await self.create_crd()
            else:
                logger.error(f"Error checking CRD existence: {e}")
                raise

    async def create_crd(self) -> bool:
        try:
            crd = CRDGenerator.generate_crd()

            self.api_client.create_custom_resource_definition(body=crd)
            logger.info(f"Successfully created CRD {self.crd_name}")

            await self._wait_for_crd_established()
            return True

        except ApiException as e:
            logger.error(f"Failed to create CRD {self.crd_name}: {e}")
            raise

    async def update_crd(self) -> bool:
        try:
            crd = CRDGenerator.generate_crd()

            self.api_client.patch_custom_resource_definition(
                name=self.crd_name, body=crd
            )
            logger.info(f"Successfully updated CRD {self.crd_name}")
            return True

        except ApiException as e:
            logger.error(f"Failed to update CRD {self.crd_name}: {e}")
            raise

    async def delete_crd(self) -> bool:
        try:
            self.api_client.delete_custom_resource_definition(name=self.crd_name)
            logger.info(f"Successfully deleted CRD {self.crd_name}")
            return True

        except ApiException as e:
            if e.status == 404:
                logger.info(f"CRD {self.crd_name} not found, nothing to delete")
                return True
            else:
                logger.error(f"Failed to delete CRD {self.crd_name}: {e}")
                raise

    async def _wait_for_crd_established(self, timeout: int = 30):
        logger.info(f"Waiting for CRD {self.crd_name} to be established...")

        for _ in range(timeout):
            try:
                crd = self.api_client.read_custom_resource_definition(
                    name=self.crd_name
                )

                conditions = crd.status.conditions or []
                for condition in conditions:
                    if condition.type == "Established" and condition.status == "True":
                        logger.info(f"CRD {self.crd_name} is established")
                        return

            except ApiException:
                pass

            await asyncio.sleep(1)

        raise TimeoutError(
            f"CRD {self.crd_name} was not established within {timeout} seconds"
        )

    def generate_yaml(self) -> str:
        return CRDGenerator.generate_crd_yaml()
