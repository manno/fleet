#!/usr/bin/env bash

# Copyright 2017 The Kubernetes Authors.
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


# This script is adapted from k8s.io/sample-apiserver
# Instead of vendoring the code-generator, we use `go env GOPATH` to find the code-generator package
# SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/../../../
# CODE_GENERATOR_VERSION=$(go list -m k8s.io/code-generator | awk '{print $2}')
# GOPATH=$(go env GOPATH)
# CODEGEN_PKG="$GOPATH/pkg/mod/k8s.io/code-generator@$CODE_GENERATOR_VERSION"

# source "${CODEGEN_PKG}/kube_codegen.sh"
# echo $CODEGEN_PKG
#
# THIS_PKG="github.com/rancher/fleet"
#
# kube::codegen::gen_helpers \
#     --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
#     "${SCRIPT_ROOT}/api/storage"
#
# if [[ -n "${API_KNOWN_VIOLATIONS_DIR:-}" ]]; then
#     report_filename="${API_KNOWN_VIOLATIONS_DIR}/sample_apiserver_violation_exceptions.list"
#     if [[ "${UPDATE_API_KNOWN_VIOLATIONS:-}" == "true" ]]; then
#         update_report="--update-report"
#     fi
# fi

openapi-gen \
  --output-dir pkg/generated/openapi \
  --output-pkg github.com/rancher/fleet/pkg/generated/openapi \
  --output-file zz_generated.openapi.go \
  --go-header-file cmd/codegen/boilerplate.go.txt \
  "k8s.io/apimachinery/pkg/apis/meta/v1" \
  "k8s.io/apimachinery/pkg/runtime" \
  "k8s.io/apimachinery/pkg/version" \
  github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1 \
  github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1/summary

# kube::codegen::gen_openapi \
#     --output-dir "${SCRIPT_ROOT}/pkg/generated/openapi" \
#     --output-pkg "${THIS_PKG}/pkg/generated/openapi" \
#     --report-filename "${report_filename:-"/dev/null"}" \
#     ${update_report:+"${update_report}"} \
#     --boilerplate "${SCRIPT_ROOT}/cmd/codegen/boilerplate.go.txt" \
#     "${SCRIPT_ROOT}/pkg/apis/fleet.cattle.io/v1alpha1"
# /Users/mm/go/bin/openapi-gen -v 0 --output-file zz_generated.openapi.go --go-header-file cmd/codegen/hack/../../..//cmd/codegen/boilerplate.go.txt --output-dir cmd/codegen/hack/../../..//pkg/generated/openapi --output-pkg github.com/rancher/fleet/pkg/generated/openapi --report-filename /var/folders/yc/jv42lyxx3qz5vsh0rh7_jx0m0000gq/T/update-codegen.sh.api_violations.XXXXXX.GiuxmezcOY k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/version github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1 github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1/summary

# kube::codegen::gen_client \
#     --with-watch \
#     --with-applyconfig \
#     --output-dir "${SCRIPT_ROOT}/pkg/generated" \
#     --output-pkg "${THIS_PKG}/pkg/generated" \
#     --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
#     "${SCRIPT_ROOT}/api"
#
