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

package gather

import (
	"context"

	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/pkg/operator/deployment"
	submarinerOp "github.com/submariner-io/submariner-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

func gatherSubmariners(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, submarinerOp.GroupVersion.WithResource("submariners"), namespace, metav1.ListOptions{})
}

func gatherServiceDiscoveries(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, submarinerOp.GroupVersion.WithResource("servicediscoveries"), namespace, metav1.ListOptions{})
}

func gatherSubmarinerOperatorDeployment(ctx context.Context, info *Info, namespace string) {
	gatherDeployment(ctx, info, namespace, metav1.ListOptions{FieldSelector: fields.Set(map[string]string{
		"metadata.name": names.OperatorComponent,
	}).String()})
}

func gatherGatewayDaemonSet(ctx context.Context, info *Info, namespace string) {
	gatherDaemonSet(ctx, info, namespace, metav1.ListOptions{LabelSelector: gatewayPodLabel})
}

func gatherGatewayLBService(ctx context.Context, info *Info, namespace string) {
	gatherService(ctx, info, namespace, metav1.ListOptions{FieldSelector: fields.Set(map[string]string{
		"metadata.name": names.GatewayComponent,
	}).String()})
}

func gatherMetricsPodDaemonSet(ctx context.Context, info *Info, namespace string) {
	gatherDaemonSet(ctx, info, namespace, metav1.ListOptions{LabelSelector: metricsProxyPodLabel})
}

func gatherRouteAgentDaemonSet(ctx context.Context, info *Info, namespace string) {
	gatherDaemonSet(ctx, info, namespace, metav1.ListOptions{LabelSelector: routeagentPodLabel})
}

func gatherGlobalnetDaemonSet(ctx context.Context, info *Info, namespace string) {
	gatherDaemonSet(ctx, info, namespace, metav1.ListOptions{LabelSelector: globalnetPodLabel})
}

func gatherLighthouseAgentDeployment(ctx context.Context, info *Info, namespace string) {
	gatherDeployment(ctx, info, namespace, metav1.ListOptions{LabelSelector: "app=submariner-lighthouse-agent"})
}

func gatherLighthouseCoreDNSDeployment(ctx context.Context, info *Info, namespace string) {
	gatherDeployment(ctx, info, namespace, metav1.ListOptions{LabelSelector: "app=submariner-lighthouse-coredns"})
}

func gatherSubmarinerOperatorPodLogs(ctx context.Context, info *Info) {
	labelSelector, err := deployment.GetPodLabelSelector(ctx, info.ClientProducer.ForKubernetes(), info.OperatorNamespace())
	if err != nil {
		info.Status.Failure("Failed to obtain the operator deployment label: %s", err)
		return
	}

	if labelSelector != "" {
		gatherPodLogs(ctx, labelSelector, info)
	}
}
