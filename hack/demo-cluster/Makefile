CLUSTER := cron-demo
CONTEXT := kind-$(CLUSTER)
REPO_ROOT := $(abspath ../..)
KUBECTL := kubectl --context $(CONTEXT)

.DEFAULT_GOAL := help
.PHONY: help up apply cron chaos heal offline build status down

help:
	@echo "up       Create the named local kind cluster and sample CronJobs"
	@echo "cron     Build and inspect the demo CronJobs"
	@echo "chaos    Add accepted configuration edge cases"
	@echo "heal     Remove the edge cases"
	@echo "offline  Explain the local definitions without a cluster"
	@echo "status   Inspect the demo's Jobs and Pods with kubectl"
	@echo "down     Delete only the named demo cluster"

up:
	kind create cluster --name $(CLUSTER) --config kind.yaml
	$(MAKE) apply

apply:
	$(KUBECTL) apply -f namespace.yaml -f schedules.yaml

build:
	cd $(REPO_ROOT) && go build -o kubectl-cron .

cron: build
	$(REPO_ROOT)/kubectl-cron --context $(CONTEXT) -n cron-demo -o wide

chaos:
	$(KUBECTL) apply -f chaos.yaml

heal:
	$(KUBECTL) delete -f chaos.yaml --ignore-not-found

offline: build
	$(REPO_ROOT)/kubectl-cron explain -f schedules.yaml

status:
	$(KUBECTL) get cronjobs,jobs,pods -n cron-demo

down:
	kind delete cluster --name $(CLUSTER)
