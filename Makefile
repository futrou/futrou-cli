SCRIPTS_DIR := ./scripts

.PHONY: default install dev build build-schema build-types generate test version start

default: install dev

install:
	@$(SCRIPTS_DIR)/install.sh

dev:
	@$(SCRIPTS_DIR)/dev.sh

build:
	@$(SCRIPTS_DIR)/build.sh

build-schema:
	@$(SCRIPTS_DIR)/build-schema.sh
	
build-types:
	@$(SCRIPTS_DIR)/build-types.sh

# Backwards-compatible alias for build-types.
generate: build-types

test:
	@go test ./src/...

version:
	@$(SCRIPTS_DIR)/version.sh $(filter-out version,$(MAKECMDGOALS))

# Forward all extra targets after 'start' as arguments to the binary.
# Usage: make start login  OR  make start ARGS="serverlets list"
start:
	@$(SCRIPTS_DIR)/start.sh $(ARGS) $(filter-out start,$(MAKECMDGOALS))

# Absorb any extra targets so make doesn't error on them
%:
	@:
