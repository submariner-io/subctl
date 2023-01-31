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
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/cluster"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/cidr"
	"github.com/submariner-io/submariner/pkg/cni"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var deploymentImageOverrides = []string{}

func AddDeploymentImageOverrideFlag(flags *pflag.FlagSet) {
	flags.StringSliceVar(&deploymentImageOverrides, "image-override", nil, "override component image")
}

func Deployments(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	if clusterInfo.Submariner != nil {
		if err := checkOverlappingCIDRs(clusterInfo, status); err != nil {
			return err
		}
	}

	if err := checkPods(clusterInfo, status); err != nil {
		return err
	}

	return checkMetricsConfig(clusterInfo, status)
}

func checkOverlappingCIDRs(clusterInfo *cluster.Info, status reporter.Interface) error {
	if clusterInfo.Submariner.Spec.GlobalCIDR != "" {
		status.Start("Globalnet deployment detected - checking if globalnet CIDRs overlap")
	} else {
		status.Start("Non-Globalnet deployment detected - checking if cluster CIDRs overlap")
	}

	defer status.End()

	endpointList := &submarinerv1.EndpointList{}

	err := clusterInfo.ClientProducer.ForGeneral().List(context.TODO(), endpointList,
		controllerClient.InNamespace(clusterInfo.Submariner.Namespace))
	if err != nil {
		return status.Error(err, "Error listing the Submariner endpoints")
	}

	tracker := reporter.NewTracker(status)

	for i := range endpointList.Items {
		source := &endpointList.Items[i]

		destEndpoints := endpointList.Items[i+1:]
		for j := range destEndpoints {
			dest := &destEndpoints[j]

			// Currently we dont support multiple endpoints in a cluster, hence return an error.
			// When the corresponding support is added, this check needs to be updated.
			if source.Spec.ClusterID == dest.Spec.ClusterID {
				tracker.Failure("Found multiple Submariner endpoints (%q and %q) in cluster %q",
					source.Name, dest.Name, source.Spec.ClusterID)
				continue
			}

			for _, subnet := range dest.Spec.Subnets {
				overlap, err := cidr.IsOverlapping(source.Spec.Subnets, subnet)
				if err != nil {
					// Ideally this case will never hit, as the subnets are valid CIDRs
					tracker.Failure("Error parsing CIDR in cluster %q: %s", dest.Spec.ClusterID, err)
					continue
				}

				if overlap {
					tracker.Failure("CIDR %q in cluster %q overlaps with cluster %q (CIDRs: %v)",
						subnet, dest.Spec.ClusterID, source.Spec.ClusterID, source.Spec.Subnets)
				}
			}
		}
	}

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing overlapping CIDRs")
	}

	if clusterInfo.Submariner.Spec.GlobalCIDR != "" {
		status.Success("Clusters do not have overlapping globalnet CIDRs")
	} else {
		status.Success("Clusters do not have overlapping CIDRs")
	}

	return nil
}

func checkPods(clusterInfo *cluster.Info, status reporter.Interface) error {
	tracker := reporter.NewTracker(status)

	if clusterInfo.Submariner != nil {
		checkDaemonset(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace, "submariner-gateway", tracker)
		checkDaemonset(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace, "submariner-routeagent", tracker)

		// Check if globalnet components are deployed and running if enabled
		if clusterInfo.Submariner.Spec.GlobalCIDR != "" {
			checkDaemonset(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace, "submariner-globalnet", tracker)
		}

		// check if networkplugin syncer components are deployed and running if enabled
		if clusterInfo.Submariner.Status.NetworkPlugin == cni.OVNKubernetes {
			checkDeployment(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace,
				"submariner-networkplugin-syncer", tracker)
		}

		checkDaemonset(clusterInfo.ClientProducer.ForKubernetes(), clusterInfo.Submariner.Namespace, "submariner-metrics-proxy", tracker)
	}

	// Check if service-discovery components are deployed and running if enabled
	if clusterInfo.ServiceDiscovery != nil {
		checkDeployment(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace, "submariner-lighthouse-agent", tracker)
		checkDeployment(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace, "submariner-lighthouse-coredns", tracker)
	}

	if clusterInfo.Submariner != nil || clusterInfo.ServiceDiscovery != nil {
		checkPodsStatus(clusterInfo.ClientProducer.ForKubernetes(), constants.OperatorNamespace, tracker)
	}

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing pods")
	}

	return nil
}

func checkDeployment(k8sClient kubernetes.Interface, namespace, deploymentName string, status reporter.Interface) {
	status.Start("Checking Deployment %q", deploymentName)
	defer status.End()

	deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if err != nil {
		status.Failure("Error obtaining Deployment %q: %v", deploymentName, err)
		return
	}

	var replicas int32 = 1
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	if deployment.Status.AvailableReplicas != replicas {
		status.Failure("The desired number of replicas for Deployment %q (%d)"+
			" does not match the actual number running (%d)", deploymentName, replicas,
			deployment.Status.AvailableReplicas)
	}
}

func checkDaemonset(k8sClient kubernetes.Interface, namespace, daemonSetName string, status reporter.Interface) {
	status.Start("Checking DaemonSet %q", daemonSetName)
	defer status.End()

	daemonSet, err := k8sClient.AppsV1().DaemonSets(namespace).Get(context.TODO(), daemonSetName, metav1.GetOptions{})
	if err != nil {
		status.Failure("Error obtaining Daemonset %q: %v", daemonSetName, err)
		return
	}

	if daemonSet.Status.CurrentNumberScheduled != daemonSet.Status.DesiredNumberScheduled {
		status.Failure("The desired number of running pods for DaemonSet %q (%d)"+
			" does not match the actual number (%d)", daemonSetName, daemonSet.Status.DesiredNumberScheduled,
			daemonSet.Status.CurrentNumberScheduled)
	}
}

func checkPodsStatus(k8sClient kubernetes.Interface, namespace string, status reporter.Interface) {
	status.Start("Checking the status of all Submariner pods")
	defer status.End()

	pods, err := k8sClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s!=%s", constants.TransientLabel, constants.TrueLabel),
	})
	if err != nil {
		status.Failure("Error obtaining Pods list: %v", err)
		return
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != v1.PodRunning {
			status.Failure("Pod %q is not running. (current state is %v)", pod.Name, pod.Status.Phase)
			continue
		}

		for j := range pod.Status.ContainerStatuses {
			c := &pod.Status.ContainerStatuses[j]
			if c.RestartCount >= 5 {
				status.Warning("Pod %q has restarted %d times", pod.Name, c.RestartCount)
			}
		}
	}
}
