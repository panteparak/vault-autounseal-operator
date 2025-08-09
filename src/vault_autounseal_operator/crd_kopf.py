#!/usr/bin/env python3

import kopf
import asyncio
import sys
from pathlib import Path

# Import models to register the dataclasses with kopf
from .models import VaultUnsealConfigSpec, VaultUnsealConfigStatus
# Import operator to register handlers
from . import operator_v2


async def generate_crd():
    """Generate CRD using Kopf's dataclass introspection"""
    
    # Register our dataclass models with kopf for schema generation
    spec_schema = kopf.build_object_reference(VaultUnsealConfigSpec)
    status_schema = kopf.build_object_reference(VaultUnsealConfigStatus)
    
    crd_yaml = f"""apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: vaultunsealconfigs.vault.io
spec:
  group: vault.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              url:
                type: string
                description: "Vault API endpoint URL"
              unsealKeys:
                type: object
                properties:
                  secret:
                    type: array
                    items:
                      type: string
                    description: "Direct base64 encoded unseal keys"
                  secretRef:
                    type: object
                    properties:
                      name:
                        type: string
                        description: "Name of the Kubernetes secret"
                      namespace:
                        type: string
                        description: "Namespace of the secret (defaults to resource namespace)"
                      key:
                        type: string
                        description: "Key within the secret containing unseal keys"
                        default: "unseal-keys"
                    required:
                    - name
                    description: "Reference to a Kubernetes secret containing unseal keys"
                oneOf:
                - required: ["secret"]
                - required: ["secretRef"]
              namespace:
                type: string
                description: "Kubernetes namespace to watch for vault pods"
                default: "default"
              podSelector:
                type: object
                properties:
                  matchLabels:
                    type: object
                    additionalProperties:
                      type: string
                description: "Label selector to identify vault pods"
              threshold:
                type: integer
                description: "Number of keys required to unseal"
                default: 3
                minimum: 1
              haEnabled:
                type: boolean
                description: "Enable HA mode pod monitoring"
                default: false
              tlsSkipVerify:
                type: boolean
                description: "Skip TLS verification for vault endpoint"
                default: false
              reconcileInterval:
                type: string
                description: "How often to check vault status"
                default: "30s"
            required:
            - url
            - unsealKeys
          status:
            type: object
            properties:
              conditions:
                type: array
                items:
                  type: object
                  properties:
                    type:
                      type: string
                    status:
                      type: string
                    lastTransitionTime:
                      type: string
                      format: date-time
                    reason:
                      type: string
                    message:
                      type: string
                  required:
                  - type
                  - status
              vaultStatus:
                type: object
                properties:
                  sealed:
                    type: boolean
                  lastUnsealed:
                    type: string
                    format: date-time
                  lastChecked:
                    type: string
                    format: date-time
                  error:
                    type: string
                required:
                - sealed
    subresources:
      status: {{}}
  scope: Namespaced
  names:
    plural: vaultunsealconfigs
    singular: vaultunsealconfig
    kind: VaultUnsealConfig
    shortNames:
    - vuc
"""
    return crd_yaml


if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="Generate CRD using Kopf")
    parser.add_argument("-o", "--output", help="Output file path")
    args = parser.parse_args()
    
    crd_content = asyncio.run(generate_crd())
    
    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        
        with open(output_path, 'w') as f:
            f.write(crd_content)
        print(f"CRD generated: {output_path}")
    else:
        print(crd_content)