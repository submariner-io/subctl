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

package show

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/internal/show/table"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/pkg/images"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const maxLogLinesToScan = 20

var componentCmd = map[string][]string{
	names.RouteAgentComponent:        {"submariner-route-agent", "--version"},
	names.GatewayComponent:           {"submariner-gateway", "--version"},
	names.GlobalnetComponent:         {"submariner-globalnet", "--version"},
	names.ServiceDiscoveryComponent:  {"lighthouse-agent", "--version"},
	names.LighthouseCoreDNSComponent: {"lighthouse-coredns", "--subm-version"},
	names.OperatorComponent:          {"submariner-operator", "--version"},
	names.MetricsProxyComponent:      {"cat", "version"},
}

func printDaemonSetVersions(clusterInfo *cluster.Info, printer *table.Printer, components ...string) error {
	daemonSets := clusterInfo.ClientProducer.ForKubernetes().AppsV1().DaemonSets(constants.OperatorNamespace)

	for _, component := range components {
		daemonSet, err := daemonSets.Get(context.TODO(), component, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

			return errors.Wrapf(err, "error retrieving %s DaemonSet", component)
		}

		// The name of the function is confusing, it just parses any image repo & version
		version, repository := images.ParseOperatorImage(daemonSet.Spec.Template.Spec.Containers[0].Image)

		runningVersion, err := getVersionForComponent(clusterInfo, component,
			labels.SelectorFromSet(daemonSet.Spec.Selector.MatchLabels))
		if err != nil {
			return errors.Wrapf(err, "error retrieving running version for %s", component)
		}

		printer.Add(component, repository, version, runningVersion)
	}

	return nil
}

func printDeploymentVersions(clusterInfo *cluster.Info, printer *table.Printer, components ...string) error {
	deployments := clusterInfo.ClientProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace)

	for _, component := range components {
		deployment, err := deployments.Get(context.TODO(), component, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

			return errors.Wrapf(err, "error retrieving %s Deployment", component)
		}

		version, repository := images.ParseOperatorImage(deployment.Spec.Template.Spec.Containers[0].Image)

		runningVersion, err := getVersionForComponent(clusterInfo, component,
			labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels))
		if err != nil {
			return err
		}

		printer.Add(component, repository, version, runningVersion)
	}

	return nil
}

func getVersionForComponent(clusterInfo *cluster.Info, component string, labelSelector labels.Selector) (string, error) {
	podsClient := clusterInfo.ClientProducer.ForKubernetes().CoreV1().Pods(constants.OperatorNamespace)
	podList, err := podsClient.List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector.String()})

	if err != nil || len(podList.Items) < 1 {
		return "", errors.Wrapf(err, "failed to find pods for component %s", component)
	}

	// Try all pods
	for i := range podList.Items {
		pod := &podList.Items[i]
		podVersion := getVersionFromPodBinary(pod, clusterInfo, component)

		if podVersion != "" {
			return podVersion, nil
		}

		podVersion = getVersionFromPodLogs(pod, podsClient, component)

		if podVersion != "" {
			return podVersion, nil
		}
	}

	return "Unavailable", nil
}

func getVersionFromPodBinary(pod *corev1.Pod, clusterInfo *cluster.Info, component string) string {
	execOptions := pods.ExecOptionsFromPod(pod)
	execConfig := pods.ExecConfig{
		RestConfig: clusterInfo.RestConfig,
		ClientSet:  clusterInfo.ClientProducer.ForKubernetes(),
	}

	execOptions.Command = componentCmd[component]

	outStr, errStr, err := pods.ExecWithOptions(context.TODO(), execConfig, &execOptions)
	if err != nil {
		return ""
	}

	if component == names.MetricsProxyComponent {
		return outStr
	}

	result, found := strings.CutPrefix(errStr, fmt.Sprintf("%s version: ", component))

	if !found {
		return ""
	}

	return result
}

func getVersionFromPodLogs(pod *corev1.Pod, podClient v1.PodInterface, component string) string {
	podLogOptions := corev1.PodLogOptions{
		Container: pod.Spec.Containers[0].Name,
	}
	logRequest := podClient.GetLogs(pod.Name, &podLogOptions)
	logStream, _ := logRequest.Stream(context.TODO())

	if logStream != nil {
		logScanner := bufio.NewScanner(logStream)
		logScanner.Split(bufio.ScanLines)

		for line := 1; logScanner.Scan() && line < maxLogLinesToScan; line++ {
			result, found := strings.CutPrefix(logScanner.Text(), fmt.Sprintf("%s version: ", component))

			if found {
				return result
			}
		}
	}

	return ""
}

func Versions(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	status.Start("Showing versions")

	printer := table.Printer{Columns: []table.Column{
		{Name: "COMPONENT"},
		{Name: "REPOSITORY"},
		{Name: "CONFIGURED"},
		{Name: "RUNNING"},
	}}

	err := printDaemonSetVersions(clusterInfo, &printer, names.GatewayComponent, names.RouteAgentComponent, names.GlobalnetComponent,
		names.MetricsProxyComponent)
	if err != nil {
		return status.Error(err, "Error retrieving DaemonSet versions")
	}

	err = printDeploymentVersions(
		clusterInfo, &printer, names.OperatorComponent, names.ServiceDiscoveryComponent, names.LighthouseCoreDNSComponent)
	if err != nil {
		return status.Error(err, "Error retrieving Deployment versions")
	}

	status.End()
	printer.Print()

	return nil
}
