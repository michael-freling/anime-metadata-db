# Makefile for the Go side of the repository: protobuf codegen and the local
# API server.
#
# The documentation site is a Next.js app under web/ with its own npm scripts
# (`npm run dev` / `npm run build`) and is not driven from here.

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

# --- API: protobuf codegen + local server ----------------------------------

.PHONY: generate
generate: ## Regenerate the committed Go and TypeScript clients and the API reference (needs buf; run npm install in web/ first)
	cd src/api && buf generate
	cd src/web && npm run generate:api-docs

.PHONY: api
api: ## Run the read-only Connect API server locally (defaults to :8080)
	cd src/api && go run ./cmd/api

# --- Dataset: the listing index the API serves ------------------------------

.PHONY: init
init: ## Download the pinned open-data sources into the cache
	cd src/builder && go run ./cmd/builder init

.PHONY: build-data
build-data: ## Rebuild dataset/data/ from the sources and dataset/overrides/
	cd src/builder && go run ./cmd/builder build

.PHONY: index
index: ## Regenerate dataset/data/index.tsv (run after any dataset change)
	cd src/api && go run ./cmd/index -root ../../dataset

.PHONY: index-check
index-check: ## Fail if dataset/data/index.tsv no longer matches dataset/data/
	cd src/api && go run ./cmd/index -check -root ../../dataset
