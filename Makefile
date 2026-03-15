GOPATH ?= $(shell go env GOPATH)

.PHONY: install install-cli install-ui build test vet ui

install: install-cli install-ui

install-cli:
	go install .

install-ui: ui
	ln -sf $(GOPATH)/bin/golem-ui-bundle/golem_ui $(GOPATH)/bin/golem-ui

ui:
	cd ui/flutter && flutter build linux --release
	rm -rf $(GOPATH)/bin/golem-ui-bundle
	cp -r ui/flutter/build/linux/x64/release/bundle $(GOPATH)/bin/golem-ui-bundle

build:
	go build ./...

test:
	go test -race -timeout 5m ./...

vet:
	go vet ./...
