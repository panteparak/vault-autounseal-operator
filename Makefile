.PHONY: help install dev test lint format generate-crd install-crd build docker-build docker-push deploy clean

# Default target
help:
	@echo "Available commands:"
	@echo "  install      - Install the package"
	@echo "  dev          - Install in development mode"
	@echo "  test         - Run tests"
	@echo "  lint         - Run code linting"
	@echo "  format       - Format code"
	@echo "  generate-crd - Generate CRD YAML file"
	@echo "  install-crd  - Install CRD to cluster"
	@echo "  build        - Build the operator"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-push  - Push Docker image"
	@echo "  deploy       - Deploy to Kubernetes"
	@echo "  clean        - Clean build artifacts"

# Installation
install:
	uv pip install .

dev:
	uv pip install -e ".[dev]"

# Testing and quality
test:
	pytest tests/ -v

test-security:
	pytest tests/test_security*.py -v -m security

test-integration:
	pytest tests/test_integration.py -v -m integration

test-coverage:
	pytest tests/ --cov=src/vault_autounseal_operator --cov-report=html --cov-report=term-missing

test-all:
	pytest tests/ -v --cov=src/vault_autounseal_operator --cov-report=html --cov-report=term-missing

lint:
	ruff src/ tests/

format:
	black src/ tests/
	ruff --fix src/ tests/

security-scan:
	@echo "Running security checks..."
	@command -v bandit >/dev/null 2>&1 || (echo "Installing bandit..." && pip install bandit)
	bandit -r src/ -f json -o security-report.json || true
	@echo "Security scan complete. Check security-report.json for results."

# CRD management
generate-crd:
	vault-operator generate-crd -o manifests/crd.yaml

generate-crd-kopf:
	vault-operator generate-crd --use-kopf -o manifests/crd-kopf.yaml

install-crd:
	vault-operator install-crd

# Build and deployment
build:
	uv pip install -e .

docker-build:
	docker build -t vault-autounseal-operator:latest .

docker-push: docker-build
	docker push vault-autounseal-operator:latest

deploy:
	kubectl apply -f manifests/crd.yaml
	kubectl apply -f manifests/rbac.yaml
	kubectl apply -f manifests/deployment.yaml

# Cleanup
clean:
	rm -rf build/
	rm -rf dist/
	rm -rf *.egg-info/
	find . -type d -name __pycache__ -exec rm -rf {} +
	find . -type f -name "*.pyc" -delete
