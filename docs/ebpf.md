# eBPF 

## Set up environment

Started the project from the following article: 
https://konghq.com/blog/engineering/writing-an-ebpf-xdp-load-balancer-in-rust

Execute the following command to respectively install build essentials,
nc, ebpftools and rust.

```shell
❯ sudo apt install build-essential netcat-traditional \
    linux-tools-common linux-tools-generic "linux-tools-$(uname -r)" \
    rustup # <- rust
```

Install requirement for Rust environment:

```shell
cargo install cargo-generate bpf-linker bindgen-cli
rustup toolchain install stable
rustup toolchain install nightly --component rust-src

# To generating C bindings:
# https://aya-rs.dev/book/aya/aya-tool/
cargo install --git https://github.com/aya-rs/aya -- aya-tool 
```

Bootstrap the project:

```shell
cd rust
cargo generate --name udplb -d program_type=xdp https://github.com/aya-rs/aya-template
cd udplb
```

Run the programm:

```shell
RUST_LOG=info cargo run --config 'target."cfg(all())".runner="sudo -E"' -- --iface=lo

# In another window:
echo -n "01234567890ABCDEF" | nc -u 127.0.0.1 8080 # or ping 127.0.0.1
```


