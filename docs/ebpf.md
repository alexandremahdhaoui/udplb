# eBPF

## Toolchain

UDPLB compiles eBPF programs from C source using clang and libbpf headers.
The cilium/ebpf library's `bpf2go` tool generates Go bindings from the compiled
bytecode, allowing the Go userspace code to load and interact with eBPF maps
and programs directly.

## Prerequisites

Install system packages:

```shell
sudo apt-get install clang libbpf-dev linux-headers-$(uname -r)
```

Go 1.22+ is required. The `bpf2go` tool installs automatically during the
forge build step.

## Build

Generate Go bindings from the eBPF C source:

```shell
forge build generate-bpf
```

This compiles `internal/adapter/bpf/udplb_kern.c` and produces Go bindings
at `internal/adapter/bpf/zz_generated*.go`.

## Source Files

- `udplb_kern.c` -- main XDP program: packet validation, session lookup, header rewrite
- `udplb_kern_helpers.c` -- helper functions: `must_loadbalance` validation, checksums, `fast_hash`
