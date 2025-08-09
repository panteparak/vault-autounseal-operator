#!/usr/bin/env python3

import asyncio
import logging
import sys

import kopf
from kubernetes import config

# Import the operator to register handlers

logger = logging.getLogger(__name__)


def setup_logging(level: str = "INFO"):
    logging.basicConfig(
        level=getattr(logging, level.upper()),
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
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


async def generate_crd_with_kopf():
    """Generate CRD using Kopf's built-in functionality"""
    setup_logging()
    setup_kubernetes()

    logger.info("Generating CRD using Kopf...")

    # This will auto-generate CRDs based on registered handlers
    crd_yaml = await kopf.build_crd(
        group="vault.io",
        versions=["v1"],
        plural="vaultunsealconfigs",
        singular="vaultunsealconfig",
        kind="VaultUnsealConfig",
        shortnames=["vuc"],
        scope="Namespaced",
    )

    return crd_yaml


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Generate CRD using Kopf")
    parser.add_argument("-o", "--output", help="Output file path")
    args = parser.parse_args()

    crd_content = asyncio.run(generate_crd_with_kopf())

    if args.output:
        with open(args.output, "w") as f:
            f.write(crd_content)
        print(f"CRD generated: {args.output}")
    else:
        print(crd_content)
