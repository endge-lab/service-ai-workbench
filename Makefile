APP_ENV ?= development
ENV_FILE ?= .env.$(APP_ENV)
LOCAL_ENV_FILE ?=$(ENV_FILE).local
RUNTIME_ENV_FILE ?=$(ENV_FILE)

ifeq ($(APP_ENV),development)
ifneq ($(wildcard $(LOCAL_ENV_FILE)),)
RUNTIME_ENV_FILE := $(LOCAL_ENV_FILE)
endif
endif

-include $(ENV_FILE)
-include $(LOCAL_ENV_FILE)
export

MAIN := ./cmd/main.go
BIN := ./tmp/service-ai-workbench
WORKBENCH_VERSION := $(strip $(shell tr -d '[:space:]' < VERSION))
BUILDINFO_PACKAGE := github.com/endge-lab/service-ai-workbench/internal/buildinfo
LDFLAGS := -s -w -X $(BUILDINFO_PACKAGE).Version=$(WORKBENCH_VERSION)

.PHONY: validate-version
validate-version:
	@test -f VERSION
	@grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$$' VERSION

.PHONY: all
all: mod build test

.PHONY: mod
mod:
	go mod tidy

.PHONY: build
build: validate-version
	go build -ldflags="$(LDFLAGS)" -buildvcs=false -o $(BIN) $(MAIN)

.PHONY: run
run: validate-version
	APP_ENV=$(APP_ENV) go run -ldflags="$(LDFLAGS)" $(MAIN)

.PHONY: clean
clean:
	rm -rf tmp

.PHONY: test
test:
	go test -v ./...

.PHONY: up
up:
	APP_RUNTIME_ENV_FILE=$(RUNTIME_ENV_FILE) docker compose --env-file $(RUNTIME_ENV_FILE) up --build

.PHONY: down
down:
	APP_RUNTIME_ENV_FILE=$(RUNTIME_ENV_FILE) docker compose --env-file $(RUNTIME_ENV_FILE) down
