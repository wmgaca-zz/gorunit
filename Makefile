.PHONY: build push

DEFAULT_TARGET: build
REVISION := $(shell git rev-parse --short HEAD)
SUDO := $(shell docker info >/dev/null 2>&1 || echo sudo)
DOCKER_REPO := docker.artifactory.olx.berlin/naspersclassifieds/gorunit

ifeq ($(BUILD_NUMBER),)
	VERSION_TAG ?= $(REVISION)
else
    VERSION_TAG ?= $(REVISION)-$(BUILD_NUMBER)
endif

build:
	$(SUDO) docker build -t $(DOCKER_REPO):$(VERSION_TAG) .

push:
	$(SUDO) docker push $(DOCKER_REPO):$(VERSION_TAG)
