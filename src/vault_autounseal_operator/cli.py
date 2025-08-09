#!/usr/bin/env python3

import asyncio
import sys
import argparse
import logging
from pathlib import Path
from kubernetes import config
from kubernetes.client.rest import ApiException

from .crd_manager import CRDManager
from .crd_schema import CRDGenerator


def setup_logging(level: str = "INFO"):
    logging.basicConfig(
        level=getattr(logging, level.upper()),
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )


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


async def generate_crd_yaml(output_file: str = None, use_kopf: bool = False):
    if use_kopf:
        from .crd_kopf import generate_crd
        yaml_content = await generate_crd()
    else:
        yaml_content = CRDGenerator.generate_crd_yaml()
    
    if output_file:
        output_path = Path(output_file)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        
        with open(output_path, 'w') as f:
            f.write(yaml_content)
            
        print(f"CRD YAML generated: {output_path}")
    else:
        print(yaml_content)


async def install_crd():
    setup_kubernetes()
    manager = CRDManager()
    
    try:
        await manager.ensure_crd_exists()
        print("CRD installation completed successfully")
    except Exception as e:
        print(f"Failed to install CRD: {e}")
        sys.exit(1)


async def update_crd():
    setup_kubernetes()
    manager = CRDManager()
    
    try:
        await manager.update_crd()
        print("CRD update completed successfully")
    except Exception as e:
        print(f"Failed to update CRD: {e}")
        sys.exit(1)


async def uninstall_crd():
    setup_kubernetes()
    manager = CRDManager()
    
    try:
        await manager.delete_crd()
        print("CRD uninstallation completed successfully")
    except Exception as e:
        print(f"Failed to uninstall CRD: {e}")
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(description="Vault Auto-Unseal Operator CLI")
    parser.add_argument("--log-level", default="INFO", 
                       choices=["DEBUG", "INFO", "WARNING", "ERROR"],
                       help="Set logging level")
    
    subparsers = parser.add_subparsers(dest="command", help="Available commands")
    
    # Generate CRD YAML command
    gen_parser = subparsers.add_parser("generate-crd", 
                                      help="Generate CRD YAML file")
    gen_parser.add_argument("-o", "--output", 
                           help="Output file path (prints to stdout if not specified)")
    gen_parser.add_argument("--use-kopf", action="store_true",
                           help="Use Kopf's built-in CRD generation (experimental)")
    
    # Install CRD command
    subparsers.add_parser("install-crd", help="Install CRD to Kubernetes cluster")
    
    # Update CRD command  
    subparsers.add_parser("update-crd", help="Update existing CRD in Kubernetes cluster")
    
    # Uninstall CRD command
    subparsers.add_parser("uninstall-crd", help="Remove CRD from Kubernetes cluster")
    
    # Run operator command
    run_parser = subparsers.add_parser("run", help="Run the operator")
    
    args = parser.parse_args()
    
    setup_logging(args.log_level)
    
    if args.command == "generate-crd":
        asyncio.run(generate_crd_yaml(args.output, getattr(args, 'use_kopf', False)))
    elif args.command == "install-crd":
        asyncio.run(install_crd())
    elif args.command == "update-crd":
        asyncio.run(update_crd())
    elif args.command == "uninstall-crd":
        asyncio.run(uninstall_crd())
    elif args.command == "run":
        from .main import main as run_operator
        asyncio.run(run_operator())
    else:
        parser.print_help()


if __name__ == "__main__":
    main()