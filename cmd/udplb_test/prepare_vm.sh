#!/usr/bin/env bash

set -o pipefail
set -o errexit
set -o nounset

__usage() {
    cat <<EOF
USAGE:
    ${0}

REQUIRED ENVS:
    - BASE_IMAGE: e.g. test/.tmp.nocloud_alpine.qcow2
    - OUT_IMAGE: e.g. test/.tmp.vm%d.qcow2
EOF
}

BASE_IMAGE="${1}"
OUT_IMAGE="${2}"

qemu-img create -F qcow2 -b "${BASE_IMAGE}" -f qcow2 "${OUT_IMAGE}" 10g

# We also need to setup readonly nfs
