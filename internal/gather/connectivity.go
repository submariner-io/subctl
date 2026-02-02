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

	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	gatewayPodLabel          = "app=submariner-gateway"
	routeagentPodLabel       = "app=submariner-routeagent"
	globalnetPodLabel        = "app=submariner-globalnet"
	metricsProxyPodLabel     = "app=submariner-metrics-proxy"
	addonPodLabel            = "app=submariner-addon"
	ovnMasterPodLabelOCP     = "app=ovnkube-master"
	ovnMasterPodLabelGeneric = "name=ovnkube-master"
	ovnKubePodLabel          = "app=ovnkube-node"
)

func gatherGatewayPodLogs(ctx context.Context, info *Info) {
	gatherPodLogs(ctx, gatewayPodLabel, info)
}

func gatherMetricsProxyPodLogs(ctx context.Context, info *Info) {
	gatherPodLogs(ctx, metricsProxyPodLabel, info)
}

func gatherRouteAgentPodLogs(ctx context.Context, info *Info) {
	gatherPodLogs(ctx, routeagentPodLabel, info)
}

func gatherGlobalnetPodLogs(ctx context.Context, info *Info) {
	gatherPodLogs(ctx, globalnetPodLabel, info)
}

func gatherAddonPodLogs(ctx context.Context, info *Info) {
	gatherPodLogs(ctx, addonPodLabel, info)
}

func gatherEndpoints(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("endpoints"), namespace, v1.ListOptions{})
}

func gatherClusters(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("clusters"), namespace, v1.ListOptions{})
}

func gatherGateways(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("gateways"), namespace, v1.ListOptions{})
}

func gatherRouteAgents(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("routeagents"), namespace, v1.ListOptions{})
}

func gatherClusterGlobalEgressIPs(ctx context.Context, info *Info) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("clusterglobalegressips"), corev1.NamespaceAll,
		v1.ListOptions{})
}

func gatherGlobalEgressIPs(ctx context.Context, info *Info) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("globalegressips"), corev1.NamespaceAll, v1.ListOptions{})
}

func gatherGlobalIngressIPs(ctx context.Context, info *Info) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("globalingressips"), corev1.NamespaceAll, v1.ListOptions{})
}

func gatherGatewayRoutes(ctx context.Context, info *Info) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("gatewayroutes"), corev1.NamespaceAll, v1.ListOptions{})
}

func gatherNonGatewayRoutes(ctx context.Context, info *Info) {
	ResourcesToYAMLFile(ctx, info, submarinerv1.SchemeGroupVersion.WithResource("nongatewayroutes"), corev1.NamespaceAll, v1.ListOptions{})
}
