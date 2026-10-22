.DEFAULT_GOAL := help
.PHONY: help build test check

help:
	@echo "build     Build ./kubectl-cron"
	@echo "test      Run unit and fixture pipeline tests"
	@echo "check     Test, vet, and check formatting"

build:
	go build -o kubectl-cron .

test:
	go test ./...

check: test
	go vet ./...
	@test -z "$$(gofmt -l .)"
