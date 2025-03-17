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
# -- PLOT
# -------------------------------------------------------------- #

PLOT_PATH := ./cmd/plot

plot: $(BENCHMARK_CSV)
	$(GO_RUN) $(PLOT_PATH) $(BENCHMARK_CSV)


# -------------------------------------------------------------- #
# -- notebook
# -------------------------------------------------------------- #

GO_VERSION := $(shell go version | sed 's/.*\([0-9]\+\.[0-9]\+\.[0-9]\+\).*/\1/')

GONB_REPO    := github.com/janpfeifer/gonb
GONB_PATH    := ${GOPATH}/src/$(GONB_REPO)
GONB_VERSION := v0.10.11
GONB_IMAGE   := alexandremahdhaoui/gonb:$(GONB_VERSION)

$(GONB_REPO)/.git:
	mkdir -p $(GONB_PATH)
	cd $(GONB_PATH); \
		git clone "https://$(GONB_REPO).git" .; \
		git checkout $(GONB_VERSION)

build-notebook: $(GONB_REPO)/.git
	sed -i 's/go 1.23/go 1.24/' $(GONB_PATH)/go.mod 
	cd $(GONB_PATH) && \
		go mod tidy && \
		go get github.com/marcboeker/go-duckdb
	docker build -t $(GONB_IMAGE) --build-arg GO_VERSION=$(GO_VERSION) -f $(GONB_PATH)/Dockerfile $(GONB_PATH)

notebook: build-notebook
	docker run -it --rm -p 8888:8888 -v "$(ROOT):/notebooks" $(GONB_IMAGE)

new-notebook:
	docker run --rm -p 8888:8888 quay.io/jupyter/pytorch-notebook:latest

