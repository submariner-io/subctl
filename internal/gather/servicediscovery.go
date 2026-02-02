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

	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	lighthouseComponentsLabel = "component=submariner-lighthouse"
	k8sCoreDNSPodLabel        = "k8s-app=kube-dns"
	ocpCoreDNSPodLabel        = "dns.operator.openshift.io/daemonset-dns=default"
	internalSvcLabel          = "submariner.io/exportedServiceRef"
)

func gatherServiceDiscoveryPodLogs(ctx context.Context, info *Info) {
	gatherPodLogs(ctx, lighthouseComponentsLabel, info)
}

func gatherCoreDNSPodLogs(ctx context.Context, info *Info) {
	if isCoreDNSTypeOcp(ctx, info) {
		gatherPodLogs(ctx, ocpCoreDNSPodLabel, info, "dns")
	} else {
		gatherPodLogs(ctx, k8sCoreDNSPodLabel, info)
	}
}

func gatherServiceExports(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceexports"), namespace, metav1.ListOptions{})
}

func gatherServiceImports(ctx context.Context, info *Info, namespace string) {
	ResourcesToYAMLFile(ctx, info, gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceimports"), namespace, metav1.ListOptions{})
}

func gatherEndpointSlices(ctx context.Context, info *Info, namespace string) {
	labelMap := map[string]string{
		discoveryv1.LabelManagedBy: lhconstants.LabelValueManagedBy,
	}
	labelSelector := labels.Set(labelMap).String()

	ResourcesToYAMLFile(ctx, info, discoveryv1.SchemeGroupVersion.WithResource("endpointslices"), namespace,
		metav1.ListOptions{LabelSelector: labelSelector})
}

func gatherConfigMapCoreDNS(ctx context.Context, info *Info) {
	namespace := "kube-system"
	name := "coredns"

	if isCoreDNSTypeOcp(ctx, info) {
		namespace = "openshift-dns"
		name = "dns-default"
	}

	fieldMap := map[string]string{
		"metadata.name": name,
	}

	fieldSelector := fields.Set(fieldMap).String()

	gatherConfigMaps(ctx, info, namespace, metav1.ListOptions{FieldSelector: fieldSelector})

	// Gather custom configname for AKS type deployments
	if info.ServiceDiscovery.Spec.CoreDNSCustomConfig != nil {
		name = info.ServiceDiscovery.Spec.CoreDNSCustomConfig.ConfigMapName

		if info.ServiceDiscovery.Spec.CoreDNSCustomConfig.Namespace != "" {
			namespace = info.ServiceDiscovery.Spec.CoreDNSCustomConfig.Namespace
		}

		fieldMap := map[string]string{
			"metadata.name": name,
		}

		fieldSelector := fields.Set(fieldMap).String()

		gatherConfigMaps(ctx, info, namespace, metav1.ListOptions{FieldSelector: fieldSelector})
	}
}

// gatherLabeledServices gathers a service based on the label provided.
func gatherLabeledServices(ctx context.Context, info *Info, label string) {
	ResourcesToYAMLFile(ctx, info, corev1.SchemeGroupVersion.WithResource("services"), corev1.NamespaceAll,
		metav1.ListOptions{LabelSelector: label})
}

func gatherConfigMapLighthouseDNS(ctx context.Context, info *Info, namespace string) {
	gatherConfigMaps(ctx, info, namespace, metav1.ListOptions{LabelSelector: lighthouseComponentsLabel})
}

func isCoreDNSTypeOcp(ctx context.Context, info *Info) bool {
	pods, err := findPods(ctx, info.ClientProducer.ForKubernetes(), ocpCoreDNSPodLabel)
	return err == nil && len(pods.Items) > 0
}
