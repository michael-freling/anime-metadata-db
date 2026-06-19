# Makefile for the Hugo documentation site (docs/) using the hugo-book theme.

HUGO        ?= hugo
DOCS_DIR    ?= docs
HUGO_PORT   ?= 1313

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: deps
deps: ## Fetch the hugo-book theme via Hugo Modules
	cd $(DOCS_DIR) && $(HUGO) mod get -u

.PHONY: serve
serve: deps ## Start the Hugo dev server with live reload (http://localhost:$(HUGO_PORT))
	cd $(DOCS_DIR) && $(HUGO) server --port $(HUGO_PORT) --buildDrafts --disableFastRender

.PHONY: build
build: deps ## Build the static site into docs/public
	cd $(DOCS_DIR) && $(HUGO) --minify

.PHONY: clean
clean: ## Remove generated site output
	rm -rf $(DOCS_DIR)/public $(DOCS_DIR)/resources
