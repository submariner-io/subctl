#!/usr/bin/env bash

set -em -o pipefail

source ${DAPPER_SOURCE}/scripts/test/lib_subctl_gather_test.sh
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
    make deploy SETTINGS="$SETTINGS" using="${USING}" -o package/.image.subctl
    declare_kubeconfig
    echo "::endgroup::" 
}

function _subctl() {
    # Print GHA groups to make looking at CI output easier
    echo "::group::Running 'subctl $@'"
    "${DAPPER_SOURCE}"/cmd/bin/subctl "$@"
    echo "::endgroup::"
}

function test_subctl_gather() {
    # Print GHA groups to make looking at CI output easier
    rm -rf "$gather_out_dir"
    mkdir "$gather_out_dir"

    _subctl gather --dir "$gather_out_dir"

    echo "::group::Validating 'subctl gather'"
    ls $gather_out_dir

    for cluster in "${clusters[@]}"; do
        with_context "${cluster}" validate_gathered_files
    done

    # Broker (on the first cluster)
    with_context "${broker}" validate_broker_resources
    echo "::endgroup::"
}

### Main ###

subm_ns=submariner-operator
submariner_broker_ns=submariner-k8s-broker
load_settings
declare_kubeconfig
deploy_env_once

# Test subctl show invocations

_subctl show all

# Test subctl gather invocations

test_subctl_gather

# Test subctl diagnose invocations

_subctl diagnose all --validation-timeout 20
_subctl diagnose firewall inter-cluster --validation-timeout 20 "${KUBECONFIGS_DIR}"/kind-config-cluster1 "${KUBECONFIGS_DIR}"/kind-config-cluster2

# Test subctl benchmark invocations

_subctl benchmark latency --intra-cluster --kubecontexts cluster1
_subctl benchmark latency --kubecontexts cluster1,cluster2

_subctl benchmark throughput --intra-cluster --kubecontexts cluster1
_subctl benchmark throughput --verbose --kubecontexts cluster1,cluster2

# Test subctl cloud prepare invocations

_subctl cloud prepare generic --kubecontext cluster1

# Test subctl uninstall invocations

_subctl uninstall -y --kubeconfig "${KUBECONFIGS_DIR}"/kind-config-cluster1

