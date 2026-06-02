SHELL := /usr/bin/env bash

TARGETS ?= all

.PHONY: help bootstrap workspace pull status build test prune-branches add-subproject up down

help:
	@printf '%s\n' \
		'make bootstrap   - clone missing subprojects into this workspace' \
		'make workspace   - write go.work for local cross-repo development' \
		'make pull        - update all cloned subprojects' \
		'make status      - show status for each subproject' \
		'make build       - build every subproject' \
		'make test        - test every subproject' \
		'make prune-branches - delete local branches whose remote was deleted (merged only; keeps unmerged)' \
		'make add-subproject NAME=llm-agent-foo [LAUNCHABLE=1] - create+scaffold a new sibling repo and register it' \
		'make up          - start launchable subprojects (or TARGETS=...)' \
		'make down        - stop launchable subprojects (or TARGETS=...)'

bootstrap:
	./scripts/eco.sh bootstrap $(TARGETS)

workspace:
	./scripts/workspace.sh

pull:
	./scripts/eco.sh pull $(TARGETS)

status:
	./scripts/eco.sh status $(TARGETS)

build:
	./scripts/eco.sh build $(TARGETS)

test:
	./scripts/eco.sh test $(TARGETS)

prune-branches:
	./scripts/eco.sh prune-branches $(TARGETS)

add-subproject:
	@test -n "$(NAME)" || { echo 'usage: make add-subproject NAME=llm-agent-foo [LAUNCHABLE=1]'; exit 1; }
	./scripts/add-subproject.sh $(NAME) $(if $(filter 1,$(LAUNCHABLE)),--launchable,)

up:
	./scripts/eco.sh up $(TARGETS)

down:
	./scripts/eco.sh down $(TARGETS)
