SHELL := /bin/bash

.DEFAULT_GOAL := help

BACKEND_PORT ?= 3000
FRONTEND_PORT ?= 3001
LOG_DIR ?= ./logs
BINARY ?= ./bin/gateway-aggregator
NPM := npm --prefix web

.PHONY: help deps deps-be deps-fe dev dev-be dev-fe build build-fe build-be run run-be test audit clean

help:
	@echo "Available targets:"
	@echo "  make deps      # install backend/frontend dependencies"
	@echo "  make dev-be    # run backend in debug mode on BACKEND_PORT"
	@echo "  make dev-fe    # run frontend dev server on FRONTEND_PORT"
	@echo "  make dev       # run backend + frontend together"
	@echo "  make build     # build frontend static assets + backend binary"
	@echo "  make run       # run backend binary"
	@echo "  make test      # run backend tests"
	@echo "  make audit     # run frontend npm audit"
	@echo "  make clean     # clean build artifacts"

deps: deps-be deps-fe

deps-be:
	go mod download

deps-fe:
	$(NPM) install

dev-be:
	@mkdir -p $(LOG_DIR)
	GIN_MODE=debug PORT=$(BACKEND_PORT) go run . --port $(BACKEND_PORT) --log-dir $(LOG_DIR)

dev-fe:
	PORT=$(FRONTEND_PORT) BROWSER=none $(NPM) start

dev:
	@mkdir -p $(LOG_DIR)
	@set -euo pipefail; \
	trap 'kill 0' INT TERM EXIT; \
	GIN_MODE=debug PORT=$(BACKEND_PORT) go run . --port $(BACKEND_PORT) --log-dir $(LOG_DIR) & \
	PORT=$(FRONTEND_PORT) BROWSER=none $(NPM) start

build: build-fe build-be

build-fe:
	$(NPM) run build

build-be:
	@mkdir -p ./bin
	go build -ldflags "-s -w -X 'NewAPI-Gateway/common.Version=$$(cat VERSION)'" -o $(BINARY)

run: run-be

run-be:
	$(BINARY) --port $(BACKEND_PORT) --log-dir $(LOG_DIR)

test:
	go test ./...

audit:
	$(NPM) audit

clean:
	rm -rf ./bin
	rm -rf ./web/build
