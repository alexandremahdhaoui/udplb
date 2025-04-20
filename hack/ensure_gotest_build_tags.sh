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

declare GOTEST_FILES COMPLIANT_FILES

function __on_failure() {
    diff --color=always -u <(echo "${GOTEST_FILES}") <(echo "${COMPLIANT_FILES}")
}

trap __on_failure EXIT

GOTEST_FILES="$(find . -name '*_test.go' ! -name 'zz_generated_*')"
COMPLIANT_FILES="$(echo "${GOTEST_FILES}" |
    xargs grep -m1 '^//go:build.*' |
    sed 's/.go:.*/.go/')"

[[ "${GOTEST_FILES}" == "${COMPLIANT_FILES}" ]]

trap : EXIT
