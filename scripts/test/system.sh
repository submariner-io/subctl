#!/usr/bin/env bash

set -em -o pipefail

source ${SCRIPTS_DIR}/lib/debug_functions
source ${SCRIPTS_DIR}/lib/utils

### Main ###

load_settings
declare_kubeconfig

. ${DAPPER_SOURCE}/scripts/test/lib_subctl_gather_test.sh

test_subctl_gather

# Run subctl diagnose as a sanity check

${DAPPER_SOURCE}/cmd/bin/subctl diagnose all
${DAPPER_SOURCE}/cmd/bin/subctl diagnose firewall inter-cluster ${KUBECONFIGS_DIR}/kind-config-cluster1 ${KUBECONFIGS_DIR}/kind-config-cluster2

# Run benchmark commands for sanity checks

${DAPPER_SOURCE}/cmd/bin/subctl benchmark latency --intra-cluster ${KUBECONFIGS_DIR}/kind-config-cluster1

${DAPPER_SOURCE}/cmd/bin/subctl benchmark latency ${KUBECONFIGS_DIR}/kind-config-cluster1 ${KUBECONFIGS_DIR}/kind-config-cluster2

${DAPPER_SOURCE}/cmd/bin/subctl benchmark throughput --intra-cluster ${KUBECONFIGS_DIR}/kind-config-cluster1

${DAPPER_SOURCE}/cmd/bin/subctl benchmark throughput --verbose ${KUBECONFIGS_DIR}/kind-config-cluster1 ${KUBECONFIGS_DIR}/kind-config-cluster2

