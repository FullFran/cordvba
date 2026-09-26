# Makefile — cordvba monorepo root
#
# This orchestrates components without duplicating their build logic: every
# target delegates to the component's own Makefile, or to `uv` for the
# Python workspace (packages/eye-client, services/twin, apps/api).

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

.PHONY: test
test: ## Run tests for every component
	$(MAKE) -C apps/eye test
	uv run --all-packages pytest

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
	uv run --all-packages pytest
	uv run --all-packages ruff check .
