ROOT  := $(shell git rev-parse --show-toplevel)
NPROC := $(shell nproc)

# -------------------------------------------------------------- #
# -- TOOLS
# -------------------------------------------------------------- #

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

NOTEBOOK_IMAGE := quay.io/jupyter/pytorch-notebook:latest

notebook: $(BENCHMARK_CSV)
	docker run --rm -p 8888:8888 -v "$(ROOT):/home/jovyan" $(NOTEBOOK_IMAGE) 

