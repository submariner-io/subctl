#!/usr/bin/env bash

set -em -o pipefail

source ${SCRIPTS_DIR}/lib/debug_functions
source ${SCRIPTS_DIR}/lib/utils

### Functions ###

function deploy_env_once() {
    if with_context "${clusters[0]}" kubectl wait --for condition=Ready pods -l app=submariner-gateway -n "${subm_ns}" --timeout=3s > /dev/null 2>&1; then
        echo "Submariner already deployed, skipping deployment..."
        return
    fi

    # Print GHA groups to make looking at CI output easier
    printf "::group::Deploying the environment"
    make deploy SETTINGS="$settings" using="${USING}" -o package/.image.subctl
    declare_kubeconfig
    echo "::endgroup::" 
}

### Main ###

settings="${DAPPER_SOURCE}/.shipyard.system.yml"
subm_ns=submariner-operator
submariner_broker_ns=submariner-k8s-broker
load_settings
declare_kubeconfig
deploy_env_once

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

