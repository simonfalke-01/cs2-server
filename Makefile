# cs2-server — build & dev helpers
#
# Go control plane (orchestrator + Discord bot) and the C# plugin + game image.

SHELL := /bin/bash
GO    ?= go

CS2_IMAGE ?= cs2-server/cs2:latest
PLUGIN_DIR := plugins/SamplePlugin
PLUGIN_NAME := SamplePlugin

.PHONY: all build orchestrator bot test vet tidy image plugins clean run-orchestrator run-bot help

all: build ## Build all Go binaries

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## --- Go control plane -----------------------------------------------------

build: ## Build orchestrator and bot into ./bin
	@mkdir -p bin
	$(GO) build -o bin/orchestrator ./cmd/orchestrator
	$(GO) build -o bin/bot ./cmd/bot

orchestrator: ## Build only the orchestrator
	@mkdir -p bin && $(GO) build -o bin/orchestrator ./cmd/orchestrator

bot: ## Build only the bot
	@mkdir -p bin && $(GO) build -o bin/bot ./cmd/bot

test: ## Run unit tests with the race detector
	$(GO) test -race ./...

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

run-orchestrator: orchestrator ## Run the orchestrator (reads env)
	./bin/orchestrator

run-bot: bot ## Run the Discord bot (reads env)
	./bin/bot

## --- Docker compose (whole stack) -----------------------------------------

up: ## Build and start the stack (docker compose up -d --build)
	docker compose up -d --build

up-bot: ## Build and start the stack including the Discord bot profile
	docker compose --profile bot up -d --build

down: ## Stop the stack
	docker compose down

image: ## Build just the game image (with SwiftlyS2 + bundled plugins)
	docker compose build game-image

## --- Plugins (local build, optional) --------------------------------------

plugins: ## Publish the sample SwiftlyS2 plugin locally into ./plugins-dist/<name>/
	dotnet publish -c Release $(PLUGIN_DIR)/$(PLUGIN_NAME).csproj -o plugins-dist/$(PLUGIN_NAME)
	@echo "Published plugin to plugins-dist/$(PLUGIN_NAME)/"

## --- Housekeeping ---------------------------------------------------------

clean: ## Remove build artifacts
	rm -rf bin plugins-dist
	rm -rf $(PLUGIN_DIR)/bin $(PLUGIN_DIR)/obj
