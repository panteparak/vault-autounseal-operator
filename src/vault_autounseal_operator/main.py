#!/usr/bin/env python3

import asyncio
import logging
import sys

import kopf
from kubernetes import config


def setup_logging():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )

    kopf_logger = logging.getLogger("kopf")
    kopf_logger.setLevel(logging.WARNING)


def setup_kubernetes():
    try:
        config.load_incluster_config()
        logging.info("Using in-cluster Kubernetes configuration")
    except config.ConfigException:
        try:
            config.load_kube_config()
            logging.info("Using local Kubernetes configuration")
        except config.ConfigException:
            logging.error("Could not load Kubernetes configuration")
            sys.exit(1)


async def main():
    setup_logging()
    setup_kubernetes()

    logger = logging.getLogger(__name__)
    logger.info("Starting Vault Auto-Unseal Operator")

    from .operator import operator_instance

    try:
        # Ensure CRD exists before starting operator
        logger.info("Ensuring CRD exists...")
        await operator_instance.ensure_crd_exists()
        logger.info("CRD ready, starting operator...")

        await kopf.run(
            standalone=True, priority=100, peering_name="vault-autounseal-operator"
        )
    except KeyboardInterrupt:
        logger.info("Received shutdown signal")
    except Exception as e:
        logger.error(f"Operator failed: {e}")
        raise
    finally:
        logger.info("Shutting down Vault Auto-Unseal Operator")


if __name__ == "__main__":
    asyncio.run(main())
