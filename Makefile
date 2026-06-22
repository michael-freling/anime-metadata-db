# Makefile for the Hugo documentation site (docs/) using the hugo-book theme.
#
# The hugo-book theme requires Hugo extended >= 0.158. To avoid depending on
# whatever Hugo happens to be installed, these targets download a pinned Hugo
# extended binary into docs/.hugo/ and use that. Override with `make HUGO=hugo ...`
# to use a system Hugo instead (must be extended and new enough).

HUGO_VERSION ?= 0.163.3
DOCS_DIR     ?= docs
HUGO_PORT    ?= 1313
HUGO_BIND    ?= 0.0.0.0

# --- Pinned local Hugo (extended) -------------------------------------------
# Detect platform for the GitHub release asset.
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_S),Darwin)
  HUGO_OS   := darwin
  HUGO_ARCH := universal
else
  HUGO_OS := linux
  ifneq (,$(filter $(UNAME_M),aarch64 arm64))
    HUGO_ARCH := arm64
  else
    HUGO_ARCH := amd64
  endif
endif

HUGO_BIN ?= $(abspath $(DOCS_DIR)/.hugo/hugo-$(HUGO_VERSION))
HUGO_URL := https://github.com/gohugoio/hugo/releases/download/v$(HUGO_VERSION)/hugo_extended_$(HUGO_VERSION)_$(HUGO_OS)-$(HUGO_ARCH).tar.gz

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

# Download the pinned Hugo binary if it isn't already present.
$(HUGO_BIN):
	@echo ">> downloading Hugo extended v$(HUGO_VERSION) ($(HUGO_OS)-$(HUGO_ARCH))"
	@mkdir -p $(dir $(HUGO_BIN))
	@curl -sSL "$(HUGO_URL)" -o $(dir $(HUGO_BIN))hugo.tar.gz
	@tar -xzf $(dir $(HUGO_BIN))hugo.tar.gz -C $(dir $(HUGO_BIN)) hugo
	@mv $(dir $(HUGO_BIN))hugo $(HUGO_BIN)
	@rm -f $(dir $(HUGO_BIN))hugo.tar.gz
	@$(HUGO_BIN) version

.PHONY: hugo
hugo: $(HUGO_BIN) ## Download the pinned Hugo extended binary into docs/.hugo/

.PHONY: deps
deps: $(HUGO_BIN) ## Fetch/update the hugo-book theme via Hugo Modules
	cd $(DOCS_DIR) && $(HUGO_BIN) mod get -u

.PHONY: serve
serve: $(HUGO_BIN) ## Start the Hugo dev server with live reload (binds $(HUGO_BIND):$(HUGO_PORT))
	cd $(DOCS_DIR) && $(HUGO_BIN) server --bind $(HUGO_BIND) --port $(HUGO_PORT) --buildDrafts --disableFastRender

.PHONY: build
build: $(HUGO_BIN) ## Build the static site into docs/public
	cd $(DOCS_DIR) && $(HUGO_BIN) --minify

.PHONY: clean
clean: ## Remove generated site output (keeps the downloaded Hugo binary)
	rm -rf $(DOCS_DIR)/public $(DOCS_DIR)/resources

.PHONY: clean-all
clean-all: clean ## Also remove the downloaded Hugo binary
	rm -rf $(DOCS_DIR)/.hugo

# --- API: protobuf codegen + local server ----------------------------------

.PHONY: generate
generate: ## Regenerate the committed Connect/protobuf Go code under gen/ (needs buf)
	buf generate

.PHONY: api
api: ## Run the read-only Connect API server locally (defaults to :8080)
	go run ./cmd/api
