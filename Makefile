SHELL := /bin/sh
GO ?= go
PYTHON ?= python3
export GOCACHE ?= $(CURDIR)/.cache/go-build
export GOTOOLCHAIN := local

.PHONY: help run build fmt fmt-check test vet check plan brief worktree scope update-manifest smoke live-check
help:
	@echo 'make run | build | check | plan | brief TASK=T01 | worktree TASK=T01 BASE=main | scope TASK=T01 BASE=main'
	@echo 'make update-manifest TASK=T01 STATUS=in_progress [COMMIT=<sha-or-ref>]'

run:
	$(GO) run ./cmd/api

build:
	$(GO) build -trimpath -o bin/api ./cmd/api

fmt:
	gofmt -w cmd internal api

fmt-check:
	@files="$$(gofmt -l cmd internal api)"; test -z "$$files" || { echo "Run make fmt for: $$files"; exit 1; }

test:
	$(GO) test -race -count=1 -timeout=90s ./...

vet:
	$(GO) vet ./...

check: fmt-check vet test
	$(PYTHON) scripts/sprint.py validate
	$(PYTHON) scripts/check_architecture.py
	$(PYTHON) -m unittest discover -s scripts -p 'test_*.py' -v

plan:
	$(PYTHON) scripts/sprint.py plan

brief:
	$(PYTHON) scripts/sprint.py brief $(TASK)

BASE ?= main
worktree:
	$(PYTHON) scripts/sprint.py worktree $(TASK) --base $(BASE)

scope:
	$(PYTHON) scripts/sprint.py check-scope $(TASK) --base $(BASE)

update-manifest:
	@test -n "$(TASK)" || { echo 'TASK is required (example: TASK=T07)'; exit 2; }
	@test -n "$(STATUS)" || { echo 'STATUS is required (planned|in_progress|implemented|integrated)'; exit 2; }
	$(PYTHON) scripts/sprint.py update $(TASK) --status $(STATUS) $(if $(COMMIT),--commit $(COMMIT),)

URL ?= http://127.0.0.1:8080
smoke:
	$(PYTHON) scripts/smoke.py --url $(URL)

live-check: build
	$(PYTHON) scripts/check_live.py
