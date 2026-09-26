# Makefile — cordvba monorepo root
#
# This orchestrates components without duplicating their build logic: every
# target delegates to the component's own Makefile. Today the only component
# is apps/eye; more will be wired in here the same way as they land.

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	     /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: eye
eye: ## Build eye (apps/eye)
	$(MAKE) -C apps/eye build

.PHONY: test
test: ## Run tests for every component
	$(MAKE) -C apps/eye test

.PHONY: lint
lint: ## Lint every component
	$(MAKE) -C apps/eye lint

.PHONY: ci
ci: ## Run the full local CI pipeline for every component
	$(MAKE) -C apps/eye ci-local
