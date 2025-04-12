#!/usr/bin/env bash
# Copyright 2025 Alexandre Mahdhaoui
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o pipefail
set -o nounset

declare C_FILES GO_FILES LICENSED_FILES

function __on_failure_go() {
    diff --color=always -u <(echo "${GO_FILES}") <(echo "${LICENSED_FILES}")
}

function __on_failure_c() {
    diff --color=always -u <(echo "${C_FILES}") <(echo "${LICENSED_FILES}")
}

# /******************************************************************************
#  * GO_FILES
#  *
#  ******************************************************************************/

trap __on_failure_go EXIT

GO_FILES="$(find . -name '*.go' ! -name 'zz_generated_*')"
LICENSED_FILES="$(echo "${GO_FILES}" |
    xargs grep 'Copyright.*Alexandre Mahdhaoui' |
    sed 's/.go:.*/.go/')"

[[ "${GO_FILES}" == "${LICENSED_FILES}" ]]

# /******************************************************************************
#  * C_FILES
#  *
#  ******************************************************************************/

trap __on_failure_c EXIT

C_FILES="$(find . -name '*.c' -o -name '*.h')"
LICENSED_FILES="$(echo "${C_FILES}" |
    xargs grep -H 'Copyright.*Alexandre Mahdhaoui' |
    sed 's/\(\.c\|\.h\):.*/\1/')"

[[ "${C_FILES}" == "${LICENSED_FILES}" ]]

trap : EXIT
