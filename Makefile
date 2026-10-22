.DEFAULT_GOAL := help
.PHONY: help build test check snapshot

help:
	@echo "build     Build ./kubectl-cron"
	@echo "test      Run unit and fixture pipeline tests"
	@echo "check     Test, vet, and check formatting"
	@echo "snapshot  Build release archives without publishing"

build:
	go build -o kubectl-cron .

test:
	go test ./...

check: test
	go vet ./...
	@test -z "$$(gofmt -l .)"

snapshot:
	goreleaser release --snapshot --clean
