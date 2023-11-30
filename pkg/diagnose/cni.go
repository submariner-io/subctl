/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package diagnose

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
	submv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/cni"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const ovnKubeDBPodLabel = "ovn-db-pod=true"

var minOVNNBVersion = semver.New("6.1.0")

var supportedNetworkPlugins = []string{
	cni.Generic, cni.CanalFlannel, cni.WeaveNet,
	cni.OpenShiftSDN, cni.OVNKubernetes, cni.Calico,
	cni.KindNet,
}

var calicoGVR = schema.GroupVersionResource{
	Group:    "crd.projectcalico.org",
	Version:  "v1",
	Resource: "ippools",
}

func CNIConfig(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	mustHaveSubmariner(clusterInfo)

	status.Start("Checking Submariner support for the CNI network plugin")
	defer status.End()

	if clusterInfo.Submariner.Status.NetworkPlugin == "" {
		status.Warning("The network plugin hasn't been determined yet.")
		return nil
	}

	isSupportedPlugin := false

	for _, np := range supportedNetworkPlugins {
		if strings.EqualFold(clusterInfo.Submariner.Status.NetworkPlugin, np) {
			isSupportedPlugin = true
			break
		}
	}

	if !isSupportedPlugin {
		status.Failure("The detected CNI plugin (%q) is not supported by Submariner. Supported plugins: %v",
			clusterInfo.Submariner.Status.NetworkPlugin, supportedNetworkPlugins)
		return errors.New("unsupported CNI plugin")
	}

	if strings.EqualFold(clusterInfo.Submariner.Status.NetworkPlugin, cni.Generic) {
		status.Warning("Submariner could not detect the CNI network plugin and is using (%q) plugin."+
			" It may or may not work.", clusterInfo.Submariner.Status.NetworkPlugin)
	} else {
		status.Success("The detected CNI network plugin (%q) is supported", clusterInfo.Submariner.Status.NetworkPlugin)
	}

	if strings.EqualFold(clusterInfo.Submariner.Status.NetworkPlugin, cni.OVNKubernetes) {
		return checkOVNVersion(context.TODO(), clusterInfo, status)
	}

	return checkCalicoIPPoolsIfCalicoCNI(clusterInfo, status)
}

func checkCalicoIPPoolsIfCalicoCNI(info *cluster.Info, status reporter.Interface) error {
	if !strings.EqualFold(info.Submariner.Status.NetworkPlugin, cni.Calico) {
		return nil
	}

	status.Start("Calico CNI detected, checking if the Submariner IPPool pre-requisites are configured")
	defer status.End()

	gateways, err := info.GetGateways()
	if err != nil {
		return status.Error(err, "Error retrieving Gateways")
	}

	if len(gateways) == 0 {
		return status.Error(errors.New("no gateways detected on the cluster"), "")
	}

	client := info.ClientProducer.ForDynamic().Resource(calicoGVR)

	ippoolList, err := client.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return status.Error(err, "Error obtaining IPPools")
	}

	if len(ippoolList.Items) < 1 {
		return status.Error(errors.New("no IPPools in the cluster"), "")
	}

	tracker := reporter.NewTracker(status)
	ippools := make(map[string]unstructured.Unstructured)

	for _, pool := range ippoolList.Items {
		cidr, found, err := unstructured.NestedString(pool.Object, "spec", "cidr")
		if err != nil {
			tracker.Failure("Error extracting field cidr from IPPool %q", pool.GetName())
			continue
		}

		if !found {
			tracker.Failure("No CIDR found in IPPool %q", pool.GetName())
			continue
		}

		ippools[cidr] = pool
	}

	checkCalicoIPPools(gateways, ippools, tracker)

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing CNI")
	}

	return nil
}

func checkCalicoIPPools(gateways []submv1.Gateway, ippools map[string]unstructured.Unstructured, status reporter.Interface) {
	for i := range gateways {
		gateway := &gateways[i]
		if gateway.Status.HAStatus != submv1.HAStatusActive {
			continue
		}

		checkCalicoSubmConfig(gateway, ippools, status)
		checkCalicoEncapsulation(gateway, ippools, status)
	}
}

func checkCalicoSubmConfig(gateway *submv1.Gateway, ippools map[string]unstructured.Unstructured, status reporter.Interface) {
	for j := range gateway.Status.Connections {
		connection := &gateway.Status.Connections[j]
		for _, subnet := range connection.Endpoint.Subnets {
			ipPool, found := ippools[subnet]
			if found {
				isDisabled, err := getSpecBool(ipPool, "disabled")
				if err != nil {
					status.Failure(err.Error())
					continue
				}

				// When spec.disabled is set to true, Calico IPAM will not assign addresses from this Pool.
				// The IPPools configured for Submariner remote CIDRs should have disabled as true.
				if !isDisabled {
					status.Failure("The IPPool %q with CIDR %q for remote endpoint"+
						" %q has disabled set to false", ipPool.GetName(), subnet, connection.Endpoint.CableName)
					continue
				}

				natOutgoing, err := getSpecBool(ipPool, "natOutgoing")
				if err != nil {
					status.Failure(err.Error())
					continue
				}

				// When spec.natOutgoing is set to true, packets from K8s pods to destinations outside of
				// any Calico IP pools will be masqueraded.
				// The IPPools configured for Submariner remote CIDRs should have natOutgoing as false.
				if natOutgoing {
					status.Failure("The IPPool %q with CIDR %q for remote endpoint"+
						" %q has natOutgoing set to true", ipPool.GetName(), subnet, connection.Endpoint.CableName)
					continue
				}
			} else {
				status.Failure("Could not find any IPPool with CIDR %q for remote"+
					" endpoint %q", subnet, connection.Endpoint.CableName)
				continue
			}
		}
	}
}

func checkCalicoEncapsulation(gateway *submv1.Gateway, ippools map[string]unstructured.Unstructured, status reporter.Interface) {
	for _, subnet := range gateway.Status.LocalEndpoint.Subnets {
		ipPool, found := ippools[subnet]
		if found {
			vxlanMode, err := getSpecString(ipPool, "vxlanMode")
			if err != nil {
				status.Failure(err.Error())
				continue
			}

			// Calico supports different types of overlay networking. Currently, Submariner is validated only
			// when Calico is deployed with VXLAN encapsulation.
			if vxlanMode != "Always" {
				status.Failure("Calico IPPool %q does not seem to be configured with VXLAN overlay encapsulation.", ipPool.GetName())
				continue
			}
		}
	}
}

func getSpecBool(pool unstructured.Unstructured, key string) (bool, error) {
	// value defaults to false when not found; not finding a bool isn't an error
	value, _, err := unstructured.NestedBool(pool.Object, "spec", key)
	return value, errors.Wrap(err, "error getting spec field")
}

func getSpecString(pool unstructured.Unstructured, key string) (string, error) {
	value, found, err := unstructured.NestedString(pool.Object, "spec", key)
	if err != nil {
		return "", errors.Wrap(err, "error getting spec field")
	}

	if !found {
		return "", fmt.Errorf("%s value not found for IPPool %q", key, pool.GetName())
	}

	return value, nil
}

func mustHaveSubmariner(clusterInfo *cluster.Info) {
	if clusterInfo.Submariner == nil {
		panic("cluster.Info.Submariner field cannot be nil")
	}
}

func checkOVNVersion(ctx context.Context, info *cluster.Info, status reporter.Interface) error {
	status.Start("Checking OVN version")
	defer status.End()

	clientSet := info.ClientProducer.ForKubernetes()

	ovnPod, err := mustFindPod(ctx, clientSet, ovnKubeDBPodLabel)
	if err != nil {
		return status.Error(err, "Failed to get OVNKubeDB Pod")
	}

	ovnNBVersion, err := getOVNNBVersion(ctx, clientSet, info.RestConfig, ovnPod)
	if err != nil {
		return status.Error(err, "Failed to get ovn-nb database version")
	}

	if ovnNBVersion.LessThan(*minOVNNBVersion) {
		status.Failure("The ovn-nb database version %v is less than the minimum supported version %v", ovnNBVersion, minOVNNBVersion)
		return errors.New("unsupported ovn-nb database version")
	}

	status.Success("The ovn-nb database version %v is supported", ovnNBVersion)

	return nil
}

func mustFindPod(ctx context.Context, clientSet kubernetes.Interface, labelSelector string) (*corev1.Pod, error) {
	pods, err := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		Limit:         1,
	})
	if err != nil {
		return nil, errors.WithMessagef(err, "error listing Pods by label selector %q", labelSelector)
	}

	if len(pods.Items) == 0 {
		return nil, errors.WithMessagef(err, "no pod found with label %v", labelSelector)
	}

	return &pods.Items[0], nil
}

func getOVNNBVersion(ctx context.Context, clientSet kubernetes.Interface, config *rest.Config, pod *corev1.Pod) (*semver.Version, error) {
	containerName := ""

	for i := 0; i < len(pod.Spec.Containers); i++ {
		container := pod.Spec.Containers[i]
		// NBDB container name is nb-ovsdb [vanilla OVNK] or nbdb [OCP].
		if strings.HasPrefix(container.Name, "nb") {
			containerName = container.Name
			break
		}
	}

	cmd := []string{"ovn-nbctl", "-V"}
	req := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		Param("container", containerName)
	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	err := req.Error()
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create SPDY executor")
	}

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		return nil, errors.WithMessagef(err, "failed to execute SPDY command")
	}

	results := strings.Split(stdout.String(), "DB Schema")

	if len(results) < 2 {
		return nil, errors.WithMessagef(err, "unable to determine the version from the ovn-nbctl output: %q", stdout.String())
	}

	return semver.New(strings.TrimSpace(results[1])), nil
}
