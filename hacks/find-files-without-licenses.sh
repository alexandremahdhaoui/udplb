#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

declare GO_FILES LICENSED_FILES

function __on_failure() {
    diff --color=always -u <(echo "${GO_FILES}") <(echo "${LICENSED_FILES}")
}

trap __on_failure EXIT

GO_FILES="$(find . -name '*.go' ! -name 'zz_generated_*')"
LICENSED_FILES="$(echo "${GO_FILES}" |
    xargs grep 'Copyright.*Alexandre Mahdhaoui' |
    sed 's/.go:.*/.go/')"

[[ "${GO_FILES}" == "${LICENSED_FILES}" ]]

trap : EXIT
