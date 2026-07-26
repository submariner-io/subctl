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
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/pkg/ciliumcm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ciliumCNI matches github.com/submariner-io/submariner/pkg/cni.Cilium.
// Inline until that constant is in subctl's released submariner dependency.
const ciliumCNI = "cilium"

func checkCiliumClusterMeshPublisher(ctx context.Context, info *cluster.Info, status reporter.Interface) error {
	if !strings.EqualFold(info.Submariner.Status.NetworkPlugin, ciliumCNI) {
		return nil
	}

	status.Start("Cilium CNI detected, checking ClusterMesh-shaped publisher prerequisites")
	defer status.End()

	tracker := reporter.NewTracker(status)
	client := info.ClientProducer.ForKubernetes()

	ciliumNS := info.Submariner.Spec.CiliumNamespace
	if ciliumNS == "" {
		ciliumNS = info.Submariner.Status.CiliumNamespace
	}

	secretName := ciliumcm.ClusterMeshSecretNameOrDefault(info.Submariner.Spec.CiliumClusterMeshSecret)
	if info.Submariner.Spec.CiliumClusterMeshSecret == "" && info.Submariner.Status.CiliumClusterMeshSecret != "" {
		secretName = info.Submariner.Status.CiliumClusterMeshSecret
	}

	if ciliumNS == "" {
		tracker.Failure(
			"spec.ciliumNamespace is empty; set it to the namespace containing cilium-config " +
				"(subctl join sets this when exactly one cilium-config is found)")
	} else {
		checkCiliumClusterID(ctx, client, ciliumNS, tracker)
		checkCiliumClusterMeshPeer(ctx, client, ciliumNS, secretName, tracker)
	}

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing Cilium CNI publisher prerequisites")
	}

	status.Success("Cilium ClusterMesh-shaped publisher prerequisites look OK")

	return nil
}

func checkCiliumClusterID(ctx context.Context, client kubernetes.Interface, ciliumNS string, status reporter.Interface) {
	cm, err := client.CoreV1().ConfigMaps(ciliumNS).Get(ctx, ciliumcm.CiliumConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.Failure("ConfigMap %q not found in %q; cannot validate Cilium cluster-id",
				ciliumcm.CiliumConfigMapName, ciliumNS)

			return
		}

		status.Failure("Error reading ConfigMap %q: %v", ciliumcm.CiliumConfigMapName, err)

		return
	}

	for _, failure := range ciliumcm.LocalClusterIdentityFailures(cm.Data["cluster-id"], cm.Data["cluster-name"]) {
		status.Failure("%s", failure)
	}
}

func checkCiliumClusterMeshPeer(ctx context.Context, client kubernetes.Interface, ciliumNS, secretName string,
	status reporter.Interface,
) {
	secret, err := client.CoreV1().Secrets(ciliumNS).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.Failure("Secret %q not found in %q; operator should merge the Submariner peer",
				secretName, ciliumNS)

			return
		}

		status.Failure("Error reading Secret %q: %v", secretName, err)

		return
	}

	for _, key := range ciliumcm.PeerSecretKeys(ciliumcm.DefaultRemoteName) {
		if len(secret.Data[key]) == 0 {
			status.Failure("%s is missing peer key %q", secretName, key)
		}
	}
}
