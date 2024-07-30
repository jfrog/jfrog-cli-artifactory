SHELL := /bin/bash

.DEFAULT_GOAL = build

GOCMD = go
GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)

# ----------------------------------------------------------------------------------------------------------------------
export PROJECT_DIR = $(CURDIR)

clean-mock:
	@find . -name "*_mock.go" -delete

generate: clean-mock
	$(GOCMD) generate ./...

# ----------------------------------------------------------------------------------------------------------------------


utest:
	$(GOCMD) list ./... | grep "evidence" | grep -v "/${MODULE_NAME}/service/test" | xargs $(GOCMD) test $(TEST_TAGS) -timeout=1m -count=1 $(TEST_ARGS)

test: utest