ROOT  := $(shell git rev-parse --show-toplevel)
NPROC := $(shell nproc)

# -------------------------------------------------------------- #
# -- TOOLS
# -------------------------------------------------------------- #

CONTAINER_ENGINE ?= docker

GO_RUN  := go run
GO_TEST := go test -v -cpu=$(NPROC) --timeout=180m

# -------------------------------------------------------------- #
# -- RUN BENCHMARK
# -------------------------------------------------------------- #

BENCHMARK_LOG   := .ignore.benchmark.log
BENCHMARK_FLAGS := -bench=. --benchmem --benchtime=5x -count=40
BENCHMARK_PATH  := $(ROOT)/internal/adapter/bpf/ebpf_lookuptable_test.go
RUN_BENCHMARK   := $(GO_TEST) $(BENCHMARK_FLAGS) $(BENCHMARK_PATH)

$(BENCHMARK_LOG):
	$(RUN_BENCHMARK) | tee $(BENCHMARK_LOG)

# benchmark: $(BENCHMARK_LOG)

# -------------------------------------------------------------- #
# -- PARSE
# -------------------------------------------------------------- #

BENCHMARK_CSV := .ignore.benchmark.csv
PARSE_PATH    := ./cmd/parse
RUN_PARSE     := $(GO_RUN) $(PARSE_PATH)

$(BENCHMARK_CSV): $(BENCHMARK_LOG) ## Parse benchmark logs
	$(RUN_PARSE) $(BENCHMARK_LOG) | tee $(BENCHMARK_CSV)

parse: $(BENCHMARK_CSV)

# -------------------------------------------------------------- #
# -- notebook
# -------------------------------------------------------------- #

NOTEBOOK_IMAGE := alexandremahdhaoui/notebook-rlt:latest

NEW_AUTH_TOKEN     := $(shell uuidgen)
CONTAINER_RUN_FILE := .ignore.notebook-running
CONTAINER_NAME     := jupyterlab-rlt
CONTAINER_CMD      := start-notebook.sh --NotebookApp.token='$(NEW_AUTH_TOKEN)'

build-notebook:
	$(CONTAINER_ENGINE) build -t $(NOTEBOOK_IMAGE) -f Containerfile .

$(CONTAINER_RUN_FILE): build-notebook
	$(CONTAINER_ENGINE) run -d -p 8888:8888 -v "$(ROOT):/home/jovyan/udplb" --name $(CONTAINER_NAME) $(NOTEBOOK_IMAGE) $(CONTAINER_CMD)
	@echo -n "$(NEW_AUTH_TOKEN)" > $(CONTAINER_RUN_FILE)
	@echo Running container...
	@sleep 3

notebook: $(BENCHMARK_CSV) $(CONTAINER_RUN_FILE)
	@$(CONTAINER_ENGINE) start $(CONTAINER_NAME) || true
	@xdg-open "http://localhost:8888/lab?token=$(shell cat $(CONTAINER_RUN_FILE))"
	@echo "Opening: http://localhost:8888/lab?token=$(shell cat $(CONTAINER_RUN_FILE))"

# docker container exec jupyterlab-rlt jupyter server list

rm-notebook:
	$(CONTAINER_ENGINE) ps -a | grep $(CONTAINER_NAME) | awk '{print $$1}' | xargs docker container kill || true
	$(CONTAINER_ENGINE) ps -a | grep $(CONTAINER_NAME) | awk '{print $$1}' | xargs docker rm || true
	rm $(CONTAINER_RUN_FILE)

