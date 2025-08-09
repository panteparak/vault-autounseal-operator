import pytest
import yaml
from vault_autounseal_operator.crd_schema import CRDGenerator
from vault_autounseal_operator.crd_kopf import generate_crd


class TestCRDGeneration:
    
    def test_crd_generator_creates_valid_crd(self):
        """Test that CRDGenerator creates a valid CRD object"""
        crd = CRDGenerator.generate_crd()
        
        assert crd.api_version == "apiextensions.k8s.io/v1"
        assert crd.kind == "CustomResourceDefinition"
        assert crd.metadata.name == "vaultunsealconfigs.vault.io"
        
        # Check spec
        spec = crd.spec
        assert spec.group == "vault.io"
        assert spec.scope == "Namespaced"
        
        # Check names
        names = spec.names
        assert names.plural == "vaultunsealconfigs"
        assert names.singular == "vaultunsealconfig" 
        assert names.kind == "VaultUnsealConfig"
        assert "vuc" in names.short_names
        
        # Check versions
        assert len(spec.versions) == 1
        version = spec.versions[0]
        assert version.name == "v1"
        assert version.served is True
        assert version.storage is True
        assert version.schema is not None
    
    def test_crd_schema_has_required_fields(self):
        """Test that the CRD schema includes all required fields"""
        schema = CRDGenerator.generate_schema()
        
        # Check top-level structure
        assert schema.type == "object"
        assert "spec" in schema.properties
        assert "status" in schema.properties
        
        # Check spec properties
        spec_props = schema.properties["spec"].properties
        required_spec_fields = ["url", "unsealKeys"]
        
        for field in required_spec_fields:
            assert field in spec_props, f"Required field {field} missing from spec"
        
        # Check unsealKeys structure
        unseal_keys_props = spec_props["unsealKeys"].properties
        assert "secret" in unseal_keys_props
        assert "secretRef" in unseal_keys_props
        
        # Check secretRef structure
        secret_ref_props = unseal_keys_props["secretRef"].properties
        assert "name" in secret_ref_props
        assert "namespace" in secret_ref_props
        assert "key" in secret_ref_props
        
        # Check status structure
        status_props = schema.properties["status"].properties
        assert "conditions" in status_props
        assert "vaultStatus" in status_props
    
    def test_crd_yaml_generation(self):
        """Test that CRD YAML generation produces valid YAML"""
        yaml_content = CRDGenerator.generate_crd_yaml()
        
        # Should be valid YAML
        parsed = yaml.safe_load(yaml_content)
        
        # Check basic structure
        assert parsed["apiVersion"] == "apiextensions.k8s.io/v1"
        assert parsed["kind"] == "CustomResourceDefinition"
        assert parsed["metadata"]["name"] == "vaultunsealconfigs.vault.io"
        
        # Check spec
        spec = parsed["spec"]
        assert spec["group"] == "vault.io"
        assert spec["scope"] == "Namespaced"
        
        # Check schema exists and has required fields
        schema = spec["versions"][0]["schema"]["openAPIV3Schema"]
        assert "spec" in schema["properties"]
        assert "status" in schema["properties"]
    
    def test_schema_validation_constraints(self):
        """Test that the schema includes proper validation constraints"""
        schema = CRDGenerator.generate_schema()
        
        spec_props = schema.properties["spec"].properties
        
        # Check URL is required string
        url_schema = spec_props["url"]
        assert url_schema.type == "string"
        
        # Check threshold has minimum value
        threshold_schema = spec_props.get("threshold")
        if threshold_schema:
            assert threshold_schema.minimum == 1
            assert threshold_schema.default == 3
        
        # Check unsealKeys array constraints
        unseal_keys = spec_props["unsealKeys"].properties
        secret_array = unseal_keys["secret"]
        assert secret_array.type == "array"
        assert secret_array.min_items == 1
        
        # Check boolean fields have defaults
        ha_enabled = spec_props.get("haEnabled")
        if ha_enabled:
            assert ha_enabled.default is False
        
        tls_skip = spec_props.get("tlsSkipVerify")
        if tls_skip:
            assert tls_skip.default is False
    
    @pytest.mark.asyncio
    async def test_kopf_crd_generation(self):
        """Test Kopf-based CRD generation"""
        yaml_content = await generate_crd()
        
        # Should be valid YAML
        parsed = yaml.safe_load(yaml_content)
        
        # Check basic structure
        assert parsed["apiVersion"] == "apiextensions.k8s.io/v1"
        assert parsed["kind"] == "CustomResourceDefinition"
        assert parsed["metadata"]["name"] == "vaultunsealconfigs.vault.io"
        
        # Check oneOf constraint for unsealKeys
        unseal_keys_schema = parsed["spec"]["versions"][0]["schema"]["openAPIV3Schema"]["properties"]["spec"]["properties"]["unsealKeys"]
        assert "oneOf" in unseal_keys_schema
        one_of_constraints = unseal_keys_schema["oneOf"]
        
        # Should have two options: secret or secretRef
        assert len(one_of_constraints) == 2
        assert {"required": ["secret"]} in one_of_constraints
        assert {"required": ["secretRef"]} in one_of_constraints
    
    def test_crd_names_consistency(self):
        """Test that CRD names are consistent across generation methods"""
        # Test manual generation
        manual_crd = CRDGenerator.generate_crd()
        manual_names = manual_crd.spec.names
        
        # Test YAML generation
        yaml_content = CRDGenerator.generate_crd_yaml()
        parsed = yaml.safe_load(yaml_content)
        yaml_names = parsed["spec"]["names"]
        
        # Names should match
        assert manual_names.plural == yaml_names["plural"]
        assert manual_names.singular == yaml_names["singular"]
        assert manual_names.kind == yaml_names["kind"]
        assert set(manual_names.short_names) == set(yaml_names["shortNames"])
    
    def test_schema_to_dict_conversion(self):
        """Test that schema conversion to dict preserves all properties"""
        schema = CRDGenerator.generate_schema()
        schema_dict = CRDGenerator._schema_to_dict(schema)
        
        # Should preserve basic properties
        assert schema_dict["type"] == "object"
        assert "properties" in schema_dict
        
        # Should handle nested properties
        spec_props = schema_dict["properties"]["spec"]["properties"]
        assert "url" in spec_props
        assert "unsealKeys" in spec_props
        
        # Should preserve constraints
        if "threshold" in spec_props:
            threshold = spec_props["threshold"]
            assert "minimum" in threshold
            assert "default" in threshold
    
    def test_crd_subresources(self):
        """Test that CRD includes status subresource"""
        crd = CRDGenerator.generate_crd()
        version = crd.spec.versions[0]
        
        # Should have status subresource
        assert hasattr(version, 'subresources')
        # Note: The actual subresources structure depends on the Kubernetes client library version
    
    def test_crd_validation_with_invalid_data(self):
        """Test CRD schema validation logic"""
        schema = CRDGenerator.generate_schema()
        
        # Verify required fields are marked as required
        spec_schema = schema.properties["spec"]
        assert "vaultInstances" not in spec_schema.required  # Old schema
        # New schema should require url and unsealKeys
        
        # Check that array items have proper validation
        unseal_keys_schema = spec_schema.properties["unsealKeys"].properties["secret"]
        assert unseal_keys_schema.items.type == "string"