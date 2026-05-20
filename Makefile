SHELL := /usr/bin/env bash

TARGETS ?= all

.PHONY: help bootstrap workspace pull status build test up down

help:
	@printf '%s\n' \
		'make bootstrap   - clone missing subprojects into this workspace' \
		'make workspace   - write go.work for local cross-repo development' \
		'make pull        - update all cloned subprojects' \
		'make status      - show status for each subproject' \
		'make build       - build every subproject' \
		'make test        - test every subproject' \
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

up:
	./scripts/eco.sh up $(TARGETS)

down:
	./scripts/eco.sh down $(TARGETS)
