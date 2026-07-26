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

package deploy

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/submariner-operator/pkg/ciliumcm"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	ciliumRestartAnnotation = "kubectl.kubernetes.io/restartedAt"
	ciliumRolloutTimeout    = 5 * time.Minute
)

// SetCiliumClusterIdentity updates cluster-id and cluster-name in cilium-config.
// Cilium agents must be restarted afterwards for the values to take effect.
func SetCiliumClusterIdentity(ctx context.Context, client kubernetes.Interface, ciliumNS, clusterID, clusterName string) error {
	if err := ciliumcm.ValidateLocalClusterIdentity(clusterID, clusterName); err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm, err := client.CoreV1().ConfigMaps(ciliumNS).Get(ctx, ciliumcm.CiliumConfigMapName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "get ConfigMap %q", ciliumcm.CiliumConfigMapName)
		}

		if cm.Data == nil {
			cm.Data = map[string]string{}
		}

		cm.Data["cluster-id"] = clusterID
		cm.Data["cluster-name"] = clusterName

		_, err = client.CoreV1().ConfigMaps(ciliumNS).Update(ctx, cm, metav1.UpdateOptions{})

		return errors.Wrapf(err, "update ConfigMap %q", ciliumcm.CiliumConfigMapName)
	})
}

// RestartCiliumAgents triggers a rolling restart of the Cilium agent DaemonSet and
// waits until the rollout completes. cluster-id / cluster-name are applied at agent start.
func RestartCiliumAgents(ctx context.Context, client kubernetes.Interface, ciliumNS string, status reporter.Interface) error {
	status.Start("Restarting Cilium agent DaemonSet in namespace %q", ciliumNS)
	defer status.End()

	ds, err := findCiliumDaemonSet(ctx, client, ciliumNS)
	if err != nil {
		return status.Error(err, "Error locating the Cilium DaemonSet")
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, getErr := client.AppsV1().DaemonSets(ciliumNS).Get(ctx, ds.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}

		if current.Spec.Template.Annotations == nil {
			current.Spec.Template.Annotations = map[string]string{}
		}

		current.Spec.Template.Annotations[ciliumRestartAnnotation] = time.Now().Format(time.RFC3339)

		_, updateErr := client.AppsV1().DaemonSets(ciliumNS).Update(ctx, current, metav1.UpdateOptions{})

		return updateErr
	})
	if err != nil {
		return status.Error(err, "Error restarting Cilium DaemonSet %q", ds.Name)
	}

	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, ciliumRolloutTimeout, true,
		func(ctx context.Context) (bool, error) {
			current, getErr := client.AppsV1().DaemonSets(ciliumNS).Get(ctx, ds.Name, metav1.GetOptions{})
			if getErr != nil {
				return false, getErr
			}

			if current.Status.DesiredNumberScheduled == 0 {
				return false, nil
			}

			return current.Status.UpdatedNumberScheduled == current.Status.DesiredNumberScheduled &&
				current.Status.NumberAvailable == current.Status.DesiredNumberScheduled &&
				current.Status.NumberUnavailable == 0, nil
		})
	if err != nil {
		return status.Error(err, "Timed out waiting for Cilium DaemonSet %q to finish restarting", ds.Name)
	}

	status.Success("Cilium agent DaemonSet %q restarted", ds.Name)

	return nil
}

func findCiliumDaemonSet(ctx context.Context, client kubernetes.Interface, ciliumNS string) (*appsv1.DaemonSet, error) {
	ds, err := client.AppsV1().DaemonSets(ciliumNS).Get(ctx, ciliumcm.CiliumDaemonSetName, metav1.GetOptions{})
	if err == nil {
		return ds, nil
	}

	list, listErr := client.AppsV1().DaemonSets(ciliumNS).List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=cilium",
	})
	if listErr != nil {
		return nil, errors.Wrap(listErr, "list Cilium DaemonSets")
	}

	if len(list.Items) == 0 {
		return nil, errors.Errorf("no Cilium DaemonSet named %q (or labeled k8s-app=cilium) in namespace %q",
			ciliumcm.CiliumDaemonSetName, ciliumNS)
	}

	return &list.Items[0], nil
}

// ReadCiliumClusterIdentity returns cluster-id and cluster-name from cilium-config.
func ReadCiliumClusterIdentity(ctx context.Context, client kubernetes.Interface, ciliumNS string) (string, string, error) {
	cm, err := client.CoreV1().ConfigMaps(ciliumNS).Get(ctx, ciliumcm.CiliumConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", "", errors.Wrapf(err, "get ConfigMap %q", ciliumcm.CiliumConfigMapName)
	}

	if cm.Data == nil {
		return "", "", nil
	}

	return cm.Data["cluster-id"], cm.Data["cluster-name"], nil
}

// FormatCiliumIdentityProblems returns a multi-line description for prompts.
// Callers should print this via status.Warning (yellow ⚠).
func FormatCiliumIdentityProblems(clusterID, clusterName string) string {
	failures := ciliumcm.LocalClusterIdentityFailures(clusterID, clusterName)
	if len(failures) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Problems found in ConfigMap \"cilium-config\":\n")

	for _, f := range failures {
		// Failures already name the ConfigMap; drop the redundant prefix in the list.
		b.WriteString("  - ")
		b.WriteString(strings.TrimPrefix(f, "cilium-config "))
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}
