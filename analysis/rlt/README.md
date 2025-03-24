# rlt

`RLT` stands for Robust Lookup Table.
The `RLT` algorithm is defined in `internal/adapter/bpf/ebpf_lookuptable.go`.

## Benchmark analysis

[Benchmark analysis notebook](./rlt-benchmark.ipynb)

![Benchmark analysis bar chart](./plot-stability.png)

## Robust Lookup Table: AI Model

The goal is to replace the Robust algorithm with a deep learning model.

* ![First notebook](./rlt-ai-0.ipynb)
* ![Second notebook: predict backend coordinates](./rlt-ai-1.ipynb)

This algorithm must refactored. Calculating a loss from the current output might
be difficult.

