SCRIPTS_DIR := ./scripts

.PHONY: default install dev build build-npm release generate test version start

default: install dev

install:
	@$(SCRIPTS_DIR)/install.sh

dev:
	@$(SCRIPTS_DIR)/dev.sh

build:
	@$(SCRIPTS_DIR)/build.sh

build-npm:
	@$(SCRIPTS_DIR)/build-npm.sh

release: build build-npm

generate:
	@$(SCRIPTS_DIR)/generate.sh

test:
	@go test ./src/...

version:
	@$(SCRIPTS_DIR)/version.sh

# Forward all extra targets after 'start' as arguments to the binary.
# Usage: make start login  OR  make start ARGS="serverlets list"
start:
	@$(SCRIPTS_DIR)/start.sh $(ARGS) $(filter-out start,$(MAKECMDGOALS))

# Absorb any extra targets so make doesn't error on them
%:
	@:
