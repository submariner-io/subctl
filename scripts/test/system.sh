#!/usr/bin/env bash

set -em -o pipefail

source "${DAPPER_SOURCE}"/scripts/test/lib_subctl_gather_test.sh
source "${SCRIPTS_DIR}"/lib/debug_functions
source "${SCRIPTS_DIR}"/lib/utils

### Functions ###

function deploy_env_once() {
    if with_context "${clusters[0]}" kubectl wait --for condition=Ready pods -l app=submariner-gateway -n "${subm_ns}" --timeout=3s > /dev/null 2>&1; then
        echo "Submariner already deployed, skipping deployment..."
        return
    fi

    # Print GHA groups to make looking at CI output easier
    printf "::group::Deploying the environment"
    make deploy SETTINGS="$SETTINGS" using="${USING}"
    declare_kubeconfig
    echo "::endgroup::"
}

function _subctl() {
    # Print GHA groups to make looking at CI output easier
    echo "::group::Running 'subctl $*'"
    "${DAPPER_SOURCE}"/cmd/bin/subctl "$@" || return "$?"
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

function check_service_exported() {
    local cond
    cond=$(kubectl get serviceexport nginx-demo -o=jsonpath='{.status.conditions[?(@.type=="Valid")]}')

    [[ $(jq -r .status <<< "$cond") = "True" ]] && return 0

    echo "Service not exported: $cond"

    return 1
}

function export_service() {
    echo "::group::Running & validating 'subctl export service'"

    kubectl apply -f - <<EOF
      apiVersion: v1
      kind: Service
      metadata:
        name: nginx-demo
        namespace: default
        labels:
          app: nginx-demo
      spec:
        type: ClusterIP
        selector:
          app: nginx-demo
        ports:
        - protocol: TCP
          name: http
          port: 80
          targetPort: 8080
EOF

    subctl export service --kubeconfig "${KUBECONFIGS_DIR}"/kind-config-"$cluster" --namespace default nginx-demo

    with_retries 30 sleep_on_fail 1s check_service_exported

    echo "::endgroup::"
}

function test_subctl_diagnose_in_cluster() {
    echo "::group::Validating 'subctl diagnose' in-cluster"

    job_name="submariner-diagnose"
    kubectl apply -f - <<EOF
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: ${job_name}
        namespace: ${subm_ns}
      spec:
        template:
          metadata:
            labels:
              submariner.io/transient: "true"
          spec:
            containers:
            - name: submariner-diagnose
              image: localhost:5000/subctl:local
              command: ["subctl", "diagnose", "all", "--in-cluster"]
            restartPolicy: Never
            serviceAccountName: submariner-diagnose
        backoffLimit: 0
EOF

    echo "Print diagnose logs"
    with_retries 15 sleep_on_fail 1s kubectl logs "job/${job_name}" -n "${subm_ns}" -f

    kubectl wait --for=condition=Complete --timeout=15s -n "$subm_ns" "job/${job_name}"

    kubectl delete job "${job_name}" -n "${subm_ns}"
    echo "::endgroup::"
}

### Main ###

subm_ns=submariner-operator
submariner_broker_ns=submariner-k8s-broker
load_settings
declare_kubeconfig
deploy_env_once

[[ "${LIGHTHOUSE}" != true ]] || with_context "${clusters[0]}" export_service

# Test subctl show invocations

_subctl show all | tee /dev/stderr | (sponge ||:) | grep -q 'Cluster "cluster2"'
# Single-context variants don't say 'Cluster "foo"', check what cluster is considered local
_subctl show all --context cluster1 | tee /dev/stderr | (sponge ||:) | grep -qv 'cluster2.*local'
_subctl show all --context cluster2 | tee /dev/stderr | (sponge ||:) | grep -q 'cluster2.*local'
# Multiple-context variants list the clusters, even when there's only one
_subctl show all --contexts cluster1 | tee /dev/stderr | (sponge ||:) | grep -qv 'Cluster "cluster2"'
_subctl show all --contexts cluster2 | tee /dev/stderr | (sponge ||:) | grep -q 'Cluster "cluster2"'

# Test subctl gather invocations

test_subctl_gather

# Test subctl diagnose invocations

_subctl diagnose all --validation-timeout 20
_subctl diagnose firewall inter-cluster --validation-timeout 20 --kubeconfig "${KUBECONFIGS_DIR}"/kind-config-cluster1 --remoteconfig "${KUBECONFIGS_DIR}"/kind-config-cluster2
_subctl diagnose firewall inter-cluster --validation-timeout 20 --context cluster1 --remotecontext cluster2
_subctl diagnose firewall nat-discovery --validation-timeout 20 --kubeconfig "${KUBECONFIGS_DIR}"/kind-config-cluster1 --remoteconfig "${KUBECONFIGS_DIR}"/kind-config-cluster2
_subctl diagnose firewall nat-discovery --validation-timeout 20 --context cluster1 --remotecontext cluster2
# Obsolete firewall inter-cluster variant
_subctl diagnose firewall inter-cluster --validation-timeout 20 "${KUBECONFIGS_DIR}"/kind-config-cluster1 "${KUBECONFIGS_DIR}"/kind-config-cluster2 && exit 1
# Obsolete firewall nat-discovery variant
_subctl diagnose firewall nat-discovery --validation-timeout 20 "${KUBECONFIGS_DIR}"/kind-config-cluster1 "${KUBECONFIGS_DIR}"/kind-config-cluster2 && exit 1

# Test subctl diagnose in-cluster

with_context "${clusters[0]}" test_subctl_diagnose_in_cluster

# Test subctl benchmark invocations

_subctl benchmark latency --context cluster1 | tee /dev/stderr | (sponge ||:) | grep -q 'Performing latency tests from Non-Gateway pod to Gateway pod on cluster "cluster1"'
_subctl benchmark latency --context cluster1 --tocontext cluster2 | tee /dev/stderr | (sponge ||:) | grep -qE '(Performing latency tests from Gateway pod on cluster "cluster1" to Gateway pod on cluster "cluster2"|Latency test is not supported with Globalnet enabled, skipping the test)'

_subctl benchmark throughput --context cluster1 | tee /dev/stderr | (sponge ||:) | grep -q 'Performing throughput tests from Non-Gateway pod to Gateway pod on cluster "cluster1"'
_subctl benchmark throughput --context cluster1 --tocontext cluster2

# Obsolete variant with contexts
_subctl benchmark latency --intra-cluster --kubecontexts cluster1 && exit 1
_subctl benchmark latency --kubecontexts cluster1,cluster2 && exit 1

_subctl benchmark throughput --intra-cluster --kubecontexts cluster1 && exit 1
_subctl benchmark throughput --kubecontexts cluster1,cluster2 && exit 1

# Obsolete variant with kubeconfigs
_subctl benchmark latency "${KUBECONFIGS_DIR}"/kind-config-cluster1 "${KUBECONFIGS_DIR}"/kind-config-cluster2 && exit 1

_subctl benchmark throughput "${KUBECONFIGS_DIR}"/kind-config-cluster1 "${KUBECONFIGS_DIR}"/kind-config-cluster2 && exit 1

# Test subctl cloud prepare invocations

_subctl cloud prepare generic --context cluster1
# Deprecated variant
_subctl cloud prepare generic --kubecontext cluster1 && exit 1

# Test subctl uninstall invocations

_subctl uninstall -y --context cluster2
_subctl uninstall -y --kubeconfig "${KUBECONFIGS_DIR}"/kind-config-cluster1
