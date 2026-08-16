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
	buf generate

.PHONY: api
api: ## Run the read-only Connect API server locally (defaults to :8080)
	go run ./cmd/api
