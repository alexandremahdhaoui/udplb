# ------------------------------------------------------- ENVS ------------------------------------------------------- #

PROJECT := udplb

COMMIT_SHA := $(shell git rev-parse --short HEAD)
TIMESTAMP  := $(shell date --utc --iso-8601=seconds)
VERSION    ?= $(shell git describe --tags --always --dirty)
ROOT       := $(shell git rev-parse --show-toplevel)

GO_BUILD_LDFLAGS ?= -X main.BuildTimestamp=$(TIMESTAMP) -X main.CommitSHA=$(COMMIT_SHA) -X main.Version=$(VERSION)

# ------------------------------------------------------- VERSIONS --------------------------------------------------- #

CONTROLLER_GEN_VERSION := v0.14.0
GOFUMPT_VERSION        := v0.6.0
GOLANGCI_LINT_VERSION  := v1.59.1
GOTESTSUM_VERSION      := v1.12.0
MOCKERY_VERSION        := v2.42.0
OAPI_CODEGEN_VERSION   := v2.3.0
TOOLING_VERSION        := v0.1.4

# ------------------------------------------------------- TOOLS ------------------------------------------------------ #

CONTAINER_ENGINE   ?= docker
KIND_BINARY        ?= kind
KIND_BINARY_PREFIX ?= sudo

KINDENV_ENVS := KIND_BINARY_PREFIX="$(KIND_BINARY_PREFIX)" KIND_BINARY="$(KIND_BINARY)"

CONTROLLER_GEN := go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
KINDENV        := KIND_BINARY="$(KIND_BINARY)" $(TOOLING)/kindenv@$(TOOLING_VERSION)
GO_GEN         := go generate
GOFUMPT        := go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
GOLANGCI_LINT  := go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOTESTSUM      := go run gotest.tools/gotestsum@$(GOTESTSUM_VERSION) --format pkgname
MOCKERY        := go run github.com/vektra/mockery/v2@$(MOCKERY_VERSION)
OAPI_CODEGEN   := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
BPF2GO         := go run github.com/cilium/ebpf/cmd/bpf2go

TOOLING             := go run github.com/alexandremahdhaoui/tooling/cmd
BUILD_BINARY        := GO_BUILD_LDFLAGS="$(GO_BUILD_LDFLAGS)" $(TOOLING)/build-binary@$(TOOLING_VERSION)
BUILD_CONTAINER     := CONTAINER_ENGINE="$(CONTAINER_ENGINE)" BUILD_ARGS="GO_BUILD_LDFLAGS=$(GO_BUILD_LDFLAGS)" $(TOOLING)/build-container@$(TOOLING_VERSION)
KINDENV             := KINDENV_ENVS="$(KINDENV_ENVS)" $(TOOLING)/kindenv@$(TOOLING_VERSION)
LOCAL_CONTAINER_REG := $(TOOLING)/local-container-registry@$(TOOLING_VERSION)
OAPI_CODEGEN_HELPER := OAPI_CODEGEN="$(OAPI_CODEGEN)" $(TOOLING)/oapi-codegen-helper@$(TOOLING_VERSION)
TEST_GO             := GOTESTSUM="$(GOTESTSUM)" $(TOOLING)/test-go@$(TOOLING_VERSION)
LINT_LICENSES       := ./hacks/find-files-without-licenses.sh

CLEAN_MOCKS := rm -rf ./internal/util/mocks

# ------------------------------------------------------------------------------------------------------------- #
# -- GENERATE 
# ------------------------------------------------------------------------------------------------------------- #

# -- protobuf

PROTO_FILES := $(shell find . ! -path '.*/\.*' -name "*.proto")
PROTOC_GEN_GO_OUT=--go_out=. --go_opt=paths=source_relative
PROTOC_GEN_GO_GRPC_OUT=--go-grpc_out=. --go-grpc_opt=paths=source_relative
COMPILE_PROTO_CMD = protoc $(PROTOC_GEN_GO_OUT) $(PROTOC_GEN_GO_GRPC_OUT) $<

.PHONY: FORCE_REBUILD
FORCE_REBUILD:
	@:

# Rule to compile .proto files
%.pb.go: %.proto FORCE_REBUILD
	$(COMPILE_PROTO_CMD)

# -- generate code

BPF_GOPACKAGE := bpfadapter
BPF_DIR       := $(ROOT)/internal/adapter/bpf
BPF_FILE      := $(BPF_DIR)/$(PROJECT).c
BPF2GO_OPTS   := --go-package $(BPF_GOPACKAGE) -tags linux --output-dir $(BPF_DIR) --output-stem zz_generated

.PHONY: generate
generate: $(PROTO_FILES:.proto=.pb.go) ## Generate REST API server/client code, CRDs and other go generators.
	# $(OAPI_CODEGEN_HELPER)
	GOPACKAGE=bpf $(BPF2GO) $(BPF2GO_OPTS) $(PROJECT) $(BPF_FILE)
	$(GO_GEN) "./..."
	#
	# $(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	# $(CONTROLLER_GEN) paths="./..." \
	# 	crd:generateEmbeddedObjectMeta=true \
	# 	output:crd:artifacts:config=charts/$(PROJECT)/templates/crds
	#
	# $(CONTROLLER_GEN) paths="./..." \
	# 	rbac:roleName=$(PROJECT) \
	# 	webhook \
	# 	output:rbac:dir=charts/$(PROJECT)/templates/rbac \
	# 	output:webhook:dir=charts/$(PROJECT)/templates/webhook
	#
	# $(CLEAN_MOCKS)
	# $(MOCKERY)

# ------------------------------------------------------------------------------------------------------------- #
# -- FORMAT
# ------------------------------------------------------------------------------------------------------------- #

.PHONY: fmt
fmt:
	$(GOFUMPT) -w .

# ------------------------------------------------------------------------------------------------------------- #
# -- BUILD
# ------------------------------------------------------------------------------------------------------------- #

UDPLB_DIR       := $(ROOT)/cmd/udplb
BUILD_DIR       := $(ROOT)/build
UDPLB_BUILD_DIR := $(BUILD_DIR)/$(PROJECT)
UDPLB_BIN  	    := $(UDPLB_BUILD_DIR)/$(PROJECT).o # e.g. ./build/udplb/udplb.o

.PHONY: clean
clean:
	find . -name '*.o' -exec bash -c 'echo Removing {}... && rm {} ' ';'

.PHONY: build
build: generate fmt
	mkdir -p $(UDPLB_BUILD_DIR)
	CGO_ENABLED=0 go build -o $(UDPLB_BIN) $(UDPLB_DIR)

# ------------------------------------------------------------------------------------------------------------- #
# -- RUN 
# ------------------------------------------------------------------------------------------------------------- #

.PHONY: run
run: generate
	CGO_ENABLED=0 IFNAME=wlp3s0 go run -exec 'sudo -E' .

# ------------------------------------------------------------------------------------------------------------- #
# -- LINT
# ------------------------------------------------------------------------------------------------------------- #

.PHONY: lint
lint:
	$(LINT_LICENSES)
	$(GOLANGCI_LINT) run --fix

# ------------------------------------------------------------------------------------------------------------- #
# -- TEST
# ------------------------------------------------------------------------------------------------------------- #

# -- UNIT

# -- E2E

TEST_DIR := $(ROOT)/cmd/udplb_test

.PHONY: e2e-setup
e2e-setup: generate build
	make -C $(TEST_DIR) setup

.PHONY: e2e-run
e2e-run:
	make -C $(TEST_DIR) run

.PHONY: e2e-teardown
e2e-teardown:
	make -C $(TEST_DIR) teardown

.PHONY: e2e
e2e: e2e-setup e2e-run e2e-teardown

# ------------------------------------------------------- PRE-PUSH --------------------------------------------------- #

.PHONY: githooks
githooks: ## Set up git hooks to run before a push.
	git config core.hooksPath .githooks

.PHONY: pre-push
pre-push: generate fmt lint test
	git status --porcelain

