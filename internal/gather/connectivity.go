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
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	gatewayPodLabel             = "app=submariner-gateway"
	routeagentPodLabel          = "app=submariner-routeagent"
	globalnetPodLabel           = "app=submariner-globalnet"
	networkpluginSyncerPodLabel = "app=submariner-networkplugin-syncer"
	addonPodLabel               = "app=submariner-addon"
	ovnMasterPodLabelOCP        = "app=ovnkube-master"
	ovnMasterPodLabelGeneric    = "name=ovnkube-master"
)

func gatherGatewayPodLogs(info *Info) {
	gatherPodLogs(gatewayPodLabel, info)
}

func gatherRouteAgentPodLogs(info *Info) {
	gatherPodLogs(routeagentPodLabel, info)
}

func gatherGlobalnetPodLogs(info *Info) {
	gatherPodLogs(globalnetPodLabel, info)
}

func gatherNetworkPluginSyncerPodLogs(info *Info) {
	gatherPodLogs(networkpluginSyncerPodLabel, info)
}

func gatherAddonPodLogs(info *Info) {
	gatherPodLogs(addonPodLabel, info)
}

func gatherEndpoints(info *Info, namespace string) {
	ResourcesToYAMLFile(info, submarinerv1.SchemeGroupVersion.WithKind("EndpointList"), namespace, v1.ListOptions{})
}

func gatherClusters(info *Info, namespace string) {
	ResourcesToYAMLFile(info, submarinerv1.SchemeGroupVersion.WithKind("ClusterList"), namespace, v1.ListOptions{})
}

func gatherGateways(info *Info, namespace string) {
	ResourcesToYAMLFile(info, submarinerv1.SchemeGroupVersion.WithKind("GatewayList"), namespace, v1.ListOptions{})
}

func gatherClusterGlobalEgressIPs(info *Info) {
	ResourcesToYAMLFile(info, submarinerv1.SchemeGroupVersion.WithKind("ClusterGlobalEgressIPList"), corev1.NamespaceAll, v1.ListOptions{})
}

func gatherGlobalEgressIPs(info *Info) {
	ResourcesToYAMLFile(info, submarinerv1.SchemeGroupVersion.WithKind("GlobalEgressIPList"), corev1.NamespaceAll, v1.ListOptions{})
}

func gatherGlobalIngressIPs(info *Info) {
	ResourcesToYAMLFile(info, submarinerv1.SchemeGroupVersion.WithKind("GlobalIngressIPList"), corev1.NamespaceAll, v1.ListOptions{})
}
