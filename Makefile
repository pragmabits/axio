# Makefile for axio: the library at the repository root and the axio command
# in cmd/axio, a module of its own. Run make, or make help, for the targets.
#
# Every target checks what it needs before running. The pinned tools and the
# go.work that joins the two modules are created when missing. Go, git and the
# C compiler the race detector needs come from the system, so a missing one
# stops the target and says what to install.

.DEFAULT_GOAL := help
SHELL := /bin/sh

LIBRARY := github.com/pragmabits/axio
COMMAND_DIRECTORY := cmd/axio
# ./... does not reach cmd/axio, a module of its own, so both are named.
PACKAGES := ./... ./$(COMMAND_DIRECTORY)/...
EXAMPLES := $(notdir $(wildcard examples/*))

# The tools, pinned. Each one is installed with go install into
# TOOLS_DIRECTORY under a name carrying its version, so changing a version here
# installs it on the next run and leaves the global ones alone. go install
# verifies the module against the Go checksum database and builds it with the
# local toolchain, which golangci-lint needs: a binary built with an older Go
# cannot analyze code for go 1.27.
TOOLS_DIRECTORY := $(CURDIR)/bin/tools
GOLANGCI_LINT_VERSION := v2.13.2
GOVULNCHECK_VERSION := v1.7.0
BUMPIT_VERSION := v0.4.0

GOLANGCI_LINT := $(TOOLS_DIRECTORY)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOVULNCHECK := $(TOOLS_DIRECTORY)/govulncheck-$(GOVULNCHECK_VERSION)
BUMPIT := $(TOOLS_DIRECTORY)/bumpit-$(BUMPIT_VERSION)
TOOLS := $(GOLANGCI_LINT) $(GOVULNCHECK) $(BUMPIT)

##@ General

.PHONY: help
help: ## List the targets
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@ / { printf "\n%s\n", substr($$0, 5) } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  %-14s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo

##@ Setup

.PHONY: deps
deps: require-go require-git go.work $(TOOLS) ## Check every dependency and install the missing ones
	@echo "go             $$(go env GOVERSION)"
	@echo "git            $$(git --version | cut -d ' ' -f 3)"
	@echo "workspace      go.work"
	@echo "golangci-lint  $(GOLANGCI_LINT_VERSION)"
	@echo "govulncheck    $(GOVULNCHECK_VERSION)"
	@echo "bumpit         $(BUMPIT_VERSION)"
	@if command -v $(CC) >/dev/null 2>&1; then \
		echo "C compiler     $(CC)"; \
	else \
		echo "C compiler     missing, which only make race needs"; \
	fi

.PHONY: workspace
workspace: go.work ## Create the go.work that joins the library and the command

.PHONY: tools
tools: $(TOOLS) ## Install the pinned tools into bin/tools

##@ Build

.PHONY: build
build: require-go go.work ## Compile the library, the command and the examples
	go build $(PACKAGES)

.PHONY: install
install: require-go go.work ## Install the axio command from this checkout
	go install ./$(COMMAND_DIRECTORY)

.PHONY: run
run: require-go go.work ## Run the axio command with ARGS, as in make run ARGS="render app.log"
	go run ./$(COMMAND_DIRECTORY) $(ARGS)

##@ Quality

.PHONY: fmt
fmt: require-go ## Format every Go file
	gofmt -w .

.PHONY: fmt-check
fmt-check: require-go ## Fail when a Go file is not formatted
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "not formatted, run make fmt:" >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi

.PHONY: vet
vet: require-go go.work ## Run go vet on both modules
	go vet $(PACKAGES)

.PHONY: lint
lint: require-go $(GOLANGCI_LINT) ## Lint both modules with .golangci.yml
	$(GOLANGCI_LINT) run ./...
	cd $(COMMAND_DIRECTORY) && $(GOLANGCI_LINT) run ./...

.PHONY: test
test: require-go go.work ## Run the tests of both modules, uncached
	go test -count=1 $(PACKAGES)

.PHONY: race
race: require-go require-cc go.work ## Run the tests of both modules under the race detector
	go test -race -count=1 $(PACKAGES)

.PHONY: bench
bench: require-go ## Run the library benchmarks
	go test -run='^$$' -bench=. -benchmem .

.PHONY: vuln
vuln: require-go $(GOVULNCHECK) ## Check both modules for known vulnerabilities
	$(GOVULNCHECK) ./...
	cd $(COMMAND_DIRECTORY) && $(GOVULNCHECK) ./...

.PHONY: check
check: fmt-check vet lint test race vuln ## Run every gate: format, vet, lint, tests, race detector and vulnerabilities

##@ Examples

.PHONY: examples
examples: require-go go.work ## Run every example
	@for example in $(EXAMPLES); do \
		echo "== examples/$$example"; \
		go run ./examples/$$example || exit 1; \
	done

.PHONY: example
example: require-go go.work ## Run one example, as in make example EXAMPLE=pii
	@if [ -z "$(EXAMPLE)" ]; then \
		echo "choose an example with EXAMPLE, one of: $(EXAMPLES)" >&2; \
		exit 1; \
	fi
	go run ./examples/$(EXAMPLE)

##@ Release

.PHONY: modules
modules: require-git $(BUMPIT) ## Show the current and next version of each module
	$(BUMPIT) modules --allow-dirty

.PHONY: pin-library
pin-library: require-go ## Build the command against a published library, as in make pin-library LIBRARY_VERSION=v0.2.0
	@if [ -z "$(LIBRARY_VERSION)" ]; then \
		echo "choose the library version with LIBRARY_VERSION, as in LIBRARY_VERSION=v0.2.0" >&2; \
		exit 1; \
	fi
	cd $(COMMAND_DIRECTORY) && GOWORK=off go get $(LIBRARY)@$(LIBRARY_VERSION) && GOWORK=off go mod tidy

.PHONY: tidy
tidy: require-go ## Tidy both modules against published versions, outside the workspace
	GOWORK=off go mod tidy
	cd $(COMMAND_DIRECTORY) && GOWORK=off go mod tidy

##@ Cleanup

.PHONY: clean
clean: ## Remove the installed tools
	rm -rf $(TOOLS_DIRECTORY)

# Dependency checks: what comes from the system is only checked.

.PHONY: require-go require-git require-cc
require-go:
	@command -v go >/dev/null 2>&1 || { \
		echo "go is missing: install it from https://go.dev/dl" >&2; \
		exit 1; \
	}

require-git:
	@command -v git >/dev/null 2>&1 || { \
		echo "git is missing: install it with the system package manager" >&2; \
		exit 1; \
	}

require-cc:
	@command -v $(CC) >/dev/null 2>&1 || { \
		echo "the race detector needs a C compiler ($(CC)): install gcc or clang" >&2; \
		exit 1; \
	}
	@if [ "$$(go env CGO_ENABLED)" != 1 ]; then \
		echo "the race detector needs cgo: set CGO_ENABLED=1" >&2; \
		exit 1; \
	fi

# What can be created is created when missing.

go.work: | require-go
	go work init . ./$(COMMAND_DIRECTORY)

# go-install installs the package $(2) at version $(3) as the file $(1).
define go-install
	@echo "installing $(notdir $(2)) $(3) into $(TOOLS_DIRECTORY)"
	@mkdir -p $(TOOLS_DIRECTORY)
	@GOBIN=$(TOOLS_DIRECTORY) go install $(2)@$(3)
	@mv $(TOOLS_DIRECTORY)/$(notdir $(2)) $(1)
endef

$(GOLANGCI_LINT): | require-go
	$(call go-install,$@,github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

$(GOVULNCHECK): | require-go
	$(call go-install,$@,golang.org/x/vuln/cmd/govulncheck,$(GOVULNCHECK_VERSION))

$(BUMPIT): | require-go
	$(call go-install,$@,github.com/pragmabits/bumpit,$(BUMPIT_VERSION))
