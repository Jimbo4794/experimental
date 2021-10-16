#!/usr/bin/env bash

# Copyright 2019 The Tekton Authors
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
set -o nounset
set -o pipefail

PREFIX=${GOBIN:-${GOPATH}/bin}

OLDGOFLAGS="${GOFLAGS:-}"
REPO_ROOT_DIR=$(cd $(dirname "${0}")/..; pwd)
echo ${REPO_ROOT_DIR}
(
  # To support running this script from anywhere, we have to first cd into this directory
  # so we can install the tools.
  cd "${REPO_ROOT_DIR}/hack"
  go install k8s.io/code-generator/cmd/deepcopy-gen
)
${PREFIX}/deepcopy-gen -O zz_generated.deepcopy \
  --go-header-file ${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt \
  -i github.com/tektoncd/experimental/polling/pkg/apis/poll

${PREFIX}/deepcopy-gen -O zz_generated.deepcopy \
  --go-header-file ${REPO_ROOT_DIR}/hack/boilerplate/boilerplate.go.txt \
  -i github.com/tektoncd/experimental/polling/pkg/apis/pollrun