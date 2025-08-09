from dataclasses import dataclass, field
from typing import Dict, List, Optional


@dataclass
class SecretRef:
    name: str
    namespace: Optional[str] = None
    key: str = "unseal-keys"


@dataclass
class UnsealKeys:
    secret: Optional[List[str]] = None
    secretRef: Optional[SecretRef] = None


@dataclass
class PodSelector:
    matchLabels: Dict[str, str] = field(default_factory=dict)


@dataclass
class VaultUnsealConfigSpec:
    url: str
    unsealKeys: UnsealKeys
    namespace: str = "default"
    podSelector: Optional[PodSelector] = None
    threshold: int = 3
    haEnabled: bool = False
    tlsSkipVerify: bool = False
    reconcileInterval: str = "30s"


@dataclass
class VaultStatus:
    sealed: bool
    lastUnsealed: Optional[str] = None
    lastChecked: Optional[str] = None
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
    conditions: Optional[List[Condition]] = None
    vaultStatus: Optional[VaultStatus] = None
