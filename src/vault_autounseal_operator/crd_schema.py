from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

import yaml
from kubernetes.client import (
    V1CustomResourceDefinition,
    V1CustomResourceDefinitionNames,
    V1CustomResourceDefinitionSpec,
    V1CustomResourceDefinitionVersion,
    V1CustomResourceValidation,
    V1JSONSchemaProps,
    V1ObjectMeta,
)


@dataclass
class PodSelector:
    matchLabels: Dict[str, str] = field(default_factory=dict)


@dataclass
class VaultInstance:
    name: str
    endpoint: str
    unsealKeys: List[str]
    namespace: str = "default"
    podSelector: PodSelector = field(default_factory=PodSelector)
    threshold: int = 3
    haEnabled: bool = False
    tlsSkipVerify: bool = False


@dataclass
class VaultUnsealConfigSpec:
    vaultInstances: List[VaultInstance]
    reconcileInterval: str = "30s"


@dataclass
class VaultStatus:
    name: str
    sealed: bool
    lastUnsealed: Optional[str] = None
    error: Optional[str] = None


@dataclass
class Condition:
    type: str
    status: str
    lastTransitionTime: str
    reason: str
    message: str


@dataclass
class VaultUnsealConfigStatus:
    conditions: List[Condition] = field(default_factory=list)
    vaultStatuses: List[VaultStatus] = field(default_factory=list)


class CRDGenerator:
    GROUP = "vault.io"
    VERSION = "v1"
    KIND = "VaultUnsealConfig"
    PLURAL = "vaultunsealconfigs"
    SINGULAR = "vaultunsealconfig"
    SHORT_NAMES = ["vuc"]

    @classmethod
    def generate_schema(cls) -> V1JSONSchemaProps:
        return V1JSONSchemaProps(
            type="object",
            properties={
                "spec": V1JSONSchemaProps(
                    type="object",
                    properties={
                        "vaultInstances": V1JSONSchemaProps(
                            type="array",
                            items=V1JSONSchemaProps(
                                type="object",
                                properties={
                                    "name": V1JSONSchemaProps(
                                        type="string",
                                        description="Name identifier for this vault instance",
                                    ),
                                    "endpoint": V1JSONSchemaProps(
                                        type="string",
                                        description="Vault API endpoint URL",
                                    ),
                                    "namespace": V1JSONSchemaProps(
                                        type="string",
                                        description="Kubernetes namespace to watch for vault pods",
                                        default="default",
                                    ),
                                    "podSelector": V1JSONSchemaProps(
                                        type="object",
                                        description="Label selector to identify vault pods",
                                        properties={
                                            "matchLabels": V1JSONSchemaProps(
                                                type="object",
                                                additional_properties=V1JSONSchemaProps(
                                                    type="string"
                                                ),
                                            )
                                        },
                                    ),
                                    "unsealKeys": V1JSONSchemaProps(
                                        type="array",
                                        items=V1JSONSchemaProps(type="string"),
                                        description="Base64 encoded unseal keys",
                                        min_items=1,
                                    ),
                                    "threshold": V1JSONSchemaProps(
                                        type="integer",
                                        description="Number of keys required to unseal",
                                        default=3,
                                        minimum=1,
                                    ),
                                    "haEnabled": V1JSONSchemaProps(
                                        type="boolean",
                                        description="Enable HA mode monitoring",
                                        default=False,
                                    ),
                                    "tlsSkipVerify": V1JSONSchemaProps(
                                        type="boolean",
                                        description="Skip TLS verification for vault endpoint",
                                        default=False,
                                    ),
                                },
                                required=["name", "endpoint", "unsealKeys"],
                            ),
                        ),
                        "reconcileInterval": V1JSONSchemaProps(
                            type="string",
                            description="How often to check vault status (e.g., '30s', '1m')",
                            default="30s",
                        ),
                    },
                    required=["vaultInstances"],
                ),
                "status": V1JSONSchemaProps(
                    type="object",
                    properties={
                        "conditions": V1JSONSchemaProps(
                            type="array",
                            items=V1JSONSchemaProps(
                                type="object",
                                properties={
                                    "type": V1JSONSchemaProps(type="string"),
                                    "status": V1JSONSchemaProps(type="string"),
                                    "lastTransitionTime": V1JSONSchemaProps(
                                        type="string", format="date-time"
                                    ),
                                    "reason": V1JSONSchemaProps(type="string"),
                                    "message": V1JSONSchemaProps(type="string"),
                                },
                                required=["type", "status"],
                            ),
                        ),
                        "vaultStatuses": V1JSONSchemaProps(
                            type="array",
                            items=V1JSONSchemaProps(
                                type="object",
                                properties={
                                    "name": V1JSONSchemaProps(type="string"),
                                    "sealed": V1JSONSchemaProps(type="boolean"),
                                    "lastUnsealed": V1JSONSchemaProps(
                                        type="string", format="date-time"
                                    ),
                                    "error": V1JSONSchemaProps(type="string"),
                                },
                                required=["name", "sealed"],
                            ),
                        ),
                    },
                ),
            },
        )

    @classmethod
    def generate_crd(cls) -> V1CustomResourceDefinition:
        return V1CustomResourceDefinition(
            api_version="apiextensions.k8s.io/v1",
            kind="CustomResourceDefinition",
            metadata=V1ObjectMeta(name=f"{cls.PLURAL}.{cls.GROUP}"),
            spec=V1CustomResourceDefinitionSpec(
                group=cls.GROUP,
                versions=[
                    V1CustomResourceDefinitionVersion(
                        name=cls.VERSION,
                        served=True,
                        storage=True,
                        schema=V1CustomResourceValidation(
                            open_api_v3_schema=cls.generate_schema()
                        ),
                        subresources={"status": {}},
                    )
                ],
                scope="Namespaced",
                names=V1CustomResourceDefinitionNames(
                    plural=cls.PLURAL,
                    singular=cls.SINGULAR,
                    kind=cls.KIND,
                    short_names=cls.SHORT_NAMES,
                ),
            ),
        )

    @classmethod
    def generate_crd_yaml(cls) -> str:
        crd = cls.generate_crd()

        crd_dict = {
            "apiVersion": crd.api_version,
            "kind": crd.kind,
            "metadata": {"name": crd.metadata.name},
            "spec": {
                "group": crd.spec.group,
                "versions": [
                    {
                        "name": version.name,
                        "served": version.served,
                        "storage": version.storage,
                        "schema": {
                            "openAPIV3Schema": cls._schema_to_dict(
                                version.schema.open_api_v3_schema
                            )
                        },
                        "subresources": {"status": {}},
                    }
                    for version in crd.spec.versions
                ],
                "scope": crd.spec.scope,
                "names": {
                    "plural": crd.spec.names.plural,
                    "singular": crd.spec.names.singular,
                    "kind": crd.spec.names.kind,
                    "shortNames": crd.spec.names.short_names,
                },
            },
        }

        return yaml.dump(crd_dict, default_flow_style=False, sort_keys=False)

    @classmethod
    def _schema_to_dict(cls, schema: V1JSONSchemaProps) -> Dict[str, Any]:
        result = {}

        if schema.type:
            result["type"] = schema.type
        if schema.description:
            result["description"] = schema.description
        if schema.default is not None:
            result["default"] = schema.default
        if schema.minimum is not None:
            result["minimum"] = schema.minimum
        if schema.min_items is not None:
            result["minItems"] = schema.min_items
        if schema.format:
            result["format"] = schema.format
        if schema.required:
            result["required"] = schema.required
        if schema.properties:
            result["properties"] = {
                key: cls._schema_to_dict(value)
                for key, value in schema.properties.items()
            }
        if schema.items:
            result["items"] = cls._schema_to_dict(schema.items)
        if schema.additional_properties:
            result["additionalProperties"] = cls._schema_to_dict(
                schema.additional_properties
            )

        return result
