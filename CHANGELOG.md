## [1.1.0](https://github.com/panteparak/vault-autounseal-operator/compare/v1.0.2...v1.1.0) (2025-08-10)

### 🚀 Features

* Restrict Pages/Helm workflow to tags only and add auto-generated docs ([c9da578](https://github.com/panteparak/vault-autounseal-operator/commit/c9da57818bd4f32b1f2588e20d07022c44b874ef))

## [1.0.2](https://github.com/panteparak/vault-autounseal-operator/compare/v1.0.1...v1.0.2) (2025-08-10)

### 🐛 Bug Fixes

* Handle empty existing index.yaml in Helm repository workflow ([015f52a](https://github.com/panteparak/vault-autounseal-operator/commit/015f52ad7776fb687b05df9b8fafe3491bec111e))

## [1.0.1](https://github.com/panteparak/vault-autounseal-operator/compare/v1.0.0...v1.0.1) (2025-08-10)

### 🐛 Bug Fixes

* Add environment to Pages deployment and preserve all Helm versions ([23babc9](https://github.com/panteparak/vault-autounseal-operator/commit/23babc9ee33c0004d429045cdc860d034212cfc2))

## [1.0.0](https://github.com/panteparak/vault-autounseal-operator/compare/v0.4.4...v1.0.0) (2025-08-10)

### ⚠ BREAKING CHANGES

* - Helm repository URL changed from /helm/ to root level
- Users need to update their helm repo add command

🤖 Generated with [Claude Code](https://claude.ai/code)

Co-Authored-By: Claude <noreply@anthropic.com>

### ♻️ Code Refactoring

* Move Helm repository to root URL without /helm/ path ([ab386ba](https://github.com/panteparak/vault-autounseal-operator/commit/ab386ba893630e310748f58ca4cc4b9f4622eab3))

### 🔧 Maintenance

* **release:** 0.4.3 [skip ci] ([5061809](https://github.com/panteparak/vault-autounseal-operator/commit/50618093c78d0a120b6e1057356518ff4460ca89))

## [0.4.3](https://github.com/panteparak/vault-autounseal-operator/compare/v0.4.2...v0.4.3) (2025-08-10)

### 🐛 Bug Fixes

* Improve error handling and logging in operator startup ([009da18](https://github.com/panteparak/vault-autounseal-operator/commit/009da18048f90cbb846410e31b57c8b22f0f30b8))

### 📚 Documentation

* Add quickstart guide and Docker troubleshooting ([02e3ed9](https://github.com/panteparak/vault-autounseal-operator/commit/02e3ed96eae653f779350853eae368bdda723d23))

### 🔧 Maintenance

* **release:** 0.4.1 [skip ci] ([c93bdf6](https://github.com/panteparak/vault-autounseal-operator/commit/c93bdf6fdc7db23e7d2e79f44d6d3d9058aaf977))

## [0.4.1](https://github.com/panteparak/vault-autounseal-operator/compare/v0.4.0...v0.4.1) (2025-08-10)

### 🐛 Bug Fixes

* Remove GitHub Pages environment protection to allow tag deployments ([6ed0a9e](https://github.com/panteparak/vault-autounseal-operator/commit/6ed0a9e60dc5554423904ec76559b44c3e290ccd))

### 🔧 Maintenance

* **release:** 0.3.0 [skip ci] ([e300268](https://github.com/panteparak/vault-autounseal-operator/commit/e30026866c33a8824b696e93314e210e71b90cf9))

## [0.3.0](https://github.com/panteparak/vault-autounseal-operator/compare/v0.2.0...v0.3.0) (2025-08-10)

### 🚀 Features

* Create unified GitHub Pages for documentation and Helm repository ([488577c](https://github.com/panteparak/vault-autounseal-operator/commit/488577c85cc6683145e299949be97480440c6627))

## [0.2.0](https://github.com/panteparak/vault-autounseal-operator/compare/v0.1.1...v0.2.0) (2025-08-10)

### 🚀 Features

* Add GitHub Pages Helm repository with automated publishing ([fdf898d](https://github.com/panteparak/vault-autounseal-operator/commit/fdf898d6d411b4ada34da12a0a7ab2c7a30d1cfc))

## [0.1.1](https://github.com/panteparak/vault-autounseal-operator/compare/v0.1.0...v0.1.1) (2025-08-10)

### 🐛 Bug Fixes

* Fix Makefile heredoc syntax error in release target ([d3a368e](https://github.com/panteparak/vault-autounseal-operator/commit/d3a368e8814ca50c0cde11db3337a07f3c494857))

### ♻️ Code Refactoring

* Separate Docker build and Trivy scan into distinct jobs ([c44d74c](https://github.com/panteparak/vault-autounseal-operator/commit/c44d74c07a4898ce21ab260c094fb88744f74006))
