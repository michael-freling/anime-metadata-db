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
generate: ## Regenerate the committed Go and TypeScript clients (needs buf; run npm install in web/ first)
	cd api && buf generate

.PHONY: api
api: ## Run the read-only Connect API server locally (defaults to :8080)
	cd api && go run ./cmd/api

# --- Dataset: the listing index the API serves ------------------------------

.PHONY: init
init: ## Download the pinned open-data sources into the cache
	cd builder && go run ./cmd/builder init

.PHONY: build-data
build-data: ## Rebuild data/ from the sources and the builder's overrides
	cd builder && go run ./cmd/builder build

.PHONY: propose-qids
propose-qids: ## Propose a Wikidata QID for each series that has none (review every row; never writes)
	cd builder && go run ./cmd/proposeqids

.PHONY: index
index: ## Regenerate data/index.tsv from data/ (run after any dataset change)
	cd api && go run ./cmd/index -root ..

.PHONY: index-check
index-check: ## Fail if data/index.tsv no longer matches data/
	cd api && go run ./cmd/index -check -root ..
