# udplb tests

Dependencies:
- sudo apt-get install -y libnl-3-dev
- cyaml: https://github.com/tlsa/libcyaml

## Exploration & Documentation

**tuntap**

- Kernel implementation: [drivers/net/tun.c](~/go/src/git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux/drivers/net/tun.c)
- Kernel documentation: [Documentation/networking/tuntap.rst](~/go/src/git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux/Documentation/networking/tuntap.rst)
- https://blog.cloudflare.com/virtual-networking-101-understanding-tap/
- https://backreference.org/2010/03/26/tuntap-interface-tutorial/
- https://github.com/songgao/water/blob/master/syscalls_linux.go
- Interesting `(sudo or not) unshare -nUpfCiuT ip tuntap add mode tap tap0`
  will fail with: `ioctl(TUNSETIFF): Operation not permitted`. Is it because 
  the user does not have NET_CAP_ADMIN?

**testing**
- Explore if iperf can be helpful for testing: https://iperf.fr/

## Prepare VM image for tests

```shell
MAJOR=3
MINOR=21
PATCH=2
DL_URL=https://dl-cdn.alpinelinux.org/alpine/v${MAJOR}.${MINOR}/releases/cloud/nocloud_alpine-${MAJOR}.${MINOR}.${PATCH}-x86_64-bios-tiny-r0.qcow2

curl -sfLo .tmp.nocloud_alpine.qcow2 "${DL_URL}" 
```

