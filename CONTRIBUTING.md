# Contributing

**Build, test, and contribute to a session-aware UDP load balancer.**

## Quick start

```shell
git clone https://github.com/alexandremahdhaoui/udplb.git
cd udplb
sudo apt-get install clang libbpf-dev linux-headers-$(uname -r)
forge build generate-all
forge build udplb
forge test-all
```

Requires Go 1.22+ and [forge](https://github.com/alexandremahdhaoui/forge).

## How do I structure commits?

Each commit uses an emoji prefix and a structured body.

| Emoji | Meaning |
|-------|---------|
| ✨    | New feature |
| 🐛    | Bug fix |
| 📖    | Documentation |
| 🌱    | Minor change, refactor, or work-in-progress |

```text
✨ Short imperative summary (50 chars or less)

Why: Explain the motivation. What problem exists?

How: Describe the approach. What strategy did you choose?

What:

- pkg/foo/bar.go: description of change
- cmd/baz/main.go: description of change

How changes were verified:

- Unit tests for new logic (go test)
- forge test-all: all stages passed

Signed-off-by: Name <email>
```

Every commit requires `Signed-off-by`. Use `git commit -s` to add it automatically.

## How do I submit a pull request?

1. Create a feature branch: `git checkout -b feat/my-change`.
2. Run `forge test-all` and confirm all 5 stages pass.
3. Push and open a PR against `main`.

PR description format:

```text
## Summary
- 1-3 bullet points

## Test plan
- [ ] forge test-all passes
- [ ] Manual verification steps (if applicable)
```

## How do I run tests?

| Stage            | Command                       | What it checks                              |
|------------------|-------------------------------|---------------------------------------------|
| `lint-tags`      | `forge test run lint-tags`    | Build tag presence on all test files         |
| `lint-licenses`  | `forge test run lint-licenses`| Apache 2.0 license headers on `.go`/`.c`/`.h` |
| `lint`           | `forge test run lint`         | golangci-lint                                |
| `unit`           | `forge test run unit`         | Unit tests                                   |
| `e2e`            | `forge test run e2e`          | 3 Ubuntu 24.04 VMs via libvirt (testenv-vm)  |

Run all stages sequentially (stops on first failure):

```shell
forge test-all
```

Use `forge test run unit` and `forge test run lint` for fast iteration during development. Save `forge test-all` for final validation before pushing.

### Test environment management

E2E tests provision 3 VMs (`vm0-lb`, `vm1-lb`, `vm2-lb`) on a NAT network (`192.168.200.0/24`). They require libvirt and [testenv-vm](https://github.com/alexandremahdhaoui/testenv-vm).

```shell
forge test-create e2e           # Provision VMs without running tests
forge test-list e2e             # List active test environments
forge test-get e2e <testID>     # Inspect a test environment (files, env vars, metadata)
forge test-delete e2e <testID>  # Tear down VMs and clean up
```

Create a test environment to iterate on E2E tests without reprovisioning:

```shell
forge test-create e2e                  # Provision once
forge test run e2e --testID <testID>   # Run tests against existing environment
forge test-delete e2e <testID>         # Clean up when done
```

## How is the project structured?

```text
cmd/
  udplb/                     Entry point, bootstrap controllers
  udplb_test/                E2E test binary and setup
internal/
  adapter/
    bpf/                     eBPF program, DataStructureManager, double-buffered maps
    bgp/                     BGP route advertisement adapter
    cluster/                 Cluster topology (stubs)
    monitor/                 Backend monitoring adapter
    rlt/                     Reverse Lookup Table algorithms (6 variants)
    statemachine/            Generic state machines (array, hashmap, set, counter)
    vmagent/                 Test VM agent for E2E infrastructure
    wal/                     Write-Ahead Log implementation
  controller/
    dvds/                    Distributed Volatile Data Structure controller
  types/                     Core types, interfaces, State enum, WAL types
  util/                      WatcherMux, WorkerPool, RingBuffer, helpers
pkg/
  apis/
    common/v1alpha1/         Common protobuf definitions
    vmagent/v1alpha1/        VM agent protobuf definitions
docs/                        eBPF toolchain, ECMP references
hack/                        Shell scripts (license enforcement)
test/
  e2e/                       E2E test definitions
```

## What does each build target do?

### Code generation

| Target               | Engine            | What it does                                          |
|----------------------|-------------------|-------------------------------------------------------|
| `generate-protobuf`  | `generic-builder` | Compile `.proto` files in `pkg/apis/` to Go via protoc |
| `generate-bpf`       | `go-gen-bpf`      | Compile `udplb_kern.c` to Go bindings via bpf2go       |
| `generate-all`       | `generic-builder` | Chain: `generate-protobuf` + `generate-bpf` + `go generate` |

### Build and format

| Target   | Engine       | What it does                                 |
|----------|-------------|----------------------------------------------|
| `format` | `go-format` | Format all Go source files                    |
| `udplb`  | `go-build`  | Build binary to `./build/bin/udplb` (CGO_ENABLED=0) |

### Test stages

| Stage           | Runner              | What it does                                          |
|-----------------|---------------------|-------------------------------------------------------|
| `lint-tags`     | `go-lint-tags`      | Verify every test file has a valid build tag            |
| `lint-licenses` | `generic-test-runner` | Run `hack/ensure-licenses.sh` to check license headers |
| `lint`          | `go-lint`           | Run golangci-lint                                      |
| `unit`          | `go-test`           | Run unit tests (`//go:build unit`)                     |
| `e2e`           | `go-test` + testenv | Provision 3 VMs, run E2E tests (`//go:build e2e`)     |

## What does each package do?

### `internal/adapter/`

| Package        | Purpose                                                        |
|----------------|----------------------------------------------------------------|
| `bpf`          | Load/manage eBPF XDP program, double-buffered maps, ring buffer |
| `bgp`          | Advertise backend routes via BGP                               |
| `cluster`      | Cluster topology discovery (stubs)                             |
| `monitor`      | Monitor backend health and state transitions                   |
| `rlt`          | 6 Reverse Lookup Table algorithms for session placement        |
| `statemachine` | Generic state machines: `array[T]`, `hashmap[E,K,V]`, `set[T]`, `counter` |
| `vmagent`      | gRPC agent running inside E2E test VMs                         |
| `wal`          | Write-Ahead Log with hash chain and auto-consent               |

### `internal/controller/`, `internal/types/`, `internal/util/`

| Package      | Purpose                                                      |
|--------------|--------------------------------------------------------------|
| `controller/dvds` | Wrap a StateMachine with WAL for distributed consensus  |
| `types`      | Core types (`Backend`, `Config`, `State`), interfaces (`Runnable`, `Watcher`, `StateMachine`, `WAL`, `Cluster`) |
| `util`       | `WatcherMux`, `WorkerPool`, `RingBuffer`, helper functions   |

### `pkg/apis/`

| Package              | Purpose                               |
|----------------------|---------------------------------------|
| `common/v1alpha1`    | Common protobuf definitions           |
| `vmagent/v1alpha1`   | VM agent gRPC service definitions     |

## How do I add a new build target or test stage?

All build targets and test stages live in `forge.yaml`. Forge provides built-in engines for Go projects.

Add a build target:

```yaml
build:
  - name: my-target
    src: ./path/to/source
    dest: ./path/to/output
    engine: go://go-build       # Or go://generic-builder, go://go-gen-bpf, etc.
    spec:
      env:
        CGO_ENABLED: "0"
```

Add a test stage:

```yaml
test:
  - name: my-stage
    runner: go://go-test        # Or go://go-lint, go://generic-test-runner
```

Run `forge docs-list` for available engines. See [forge documentation](https://github.com/alexandremahdhaoui/forge) for engine-specific configuration.

## What conventions must I follow?

- **Build tags**: every Go test file requires one of `unit`, `integration`, `e2e`, `benchmark`, or `functional`. The `lint-tags` stage enforces this.
- **License headers**: every `.go`, `.c`, and `.h` file requires the Apache 2.0 header with `Copyright <year> Alexandre Mahdhaoui`. Generated files (`zz_generated_*`) are exempt. The `lint-licenses` stage enforces this.
- **Generated files**: never edit `zz_generated_*` files. Run `forge build generate-all` to regenerate.
- **Formatting**: run `forge build format` before committing. The `lint` stage catches style violations.
- **eBPF C code**: source lives in `internal/adapter/bpf/`. Compile with `forge build generate-bpf`. See [docs/ebpf.md](./docs/ebpf.md).
- **Protobuf**: `.proto` files live in `pkg/apis/`. Compile with `forge build generate-protobuf`.
- **Linting**: all lint stages must pass before merge. Run `forge test run lint-tags && forge test run lint-licenses && forge test run lint` to check locally.

## License

Apache 2.0. See [LICENSE](./LICENSE). By contributing, you agree to license your work under the same terms.
