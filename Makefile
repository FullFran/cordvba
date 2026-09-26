# Makefile — cordvba monorepo root
#
# This orchestrates components without duplicating their build logic: every
# target delegates to the component's own Makefile, or to `uv` for the
# Python workspace (packages/eye-client, services/twin, apps/api).
#
# pytest runs once per Python member rather than once across the whole
# workspace: several members share test file basenames (test_app.py,
# test_config.py, ...) and a single cross-package pytest invocation makes
# those collide in sys.modules. Ruff has no such issue and runs once.
PYTHON_MEMBERS := packages/eye-client services/twin apps/api

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	     /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: eye
eye: ## Build eye (apps/eye)
	$(MAKE) -C apps/eye build

.PHONY: twin
twin: ## Run twin's dev server (services/twin), needs EYE_BASE_URL/EYE_API_TOKEN
	uv run --package twin uvicorn twin.main:app --reload --port 8100

.PHONY: api
api: ## Run api's dev server (apps/api), needs EYE_BASE_URL/EYE_API_TOKEN/TWIN_BASE_URL/WEB_ORIGIN
	uv run --package api uvicorn api.main:app --reload --port 8000

.PHONY: test
test: ## Run tests for every component
	$(MAKE) -C apps/eye test
	@for m in $(PYTHON_MEMBERS); do \
		echo "==> pytest $$m"; \
		uv run --directory $$m pytest || exit 1; \
	done

.PHONY: lint
lint: ## Lint every component
	$(MAKE) -C apps/eye lint
	uv run --all-packages ruff check .

.PHONY: boundaries
boundaries: ## Check that nothing outside apps/eye touches eye's store or internals
	bash infra/ci/check-boundaries_test.sh
	bash infra/ci/check-boundaries.sh

.PHONY: ci
ci: boundaries ## Run the full local CI pipeline for every component
	$(MAKE) -C apps/eye ci-local
	@for m in $(PYTHON_MEMBERS); do \
		echo "==> pytest $$m"; \
		uv run --directory $$m pytest || exit 1; \
	done
	uv run --all-packages ruff check .
