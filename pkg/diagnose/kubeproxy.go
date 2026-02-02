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
	"errors"
	"strings"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner/pkg/cni"
)

const (
	kubeProxyIPVSIfaceCommand = "ip a s kube-ipvs0"
	KubeProxyMissingInterface = "ip: can't find device"
	KubeProxyNotEnabled       = "Device \"kube-ipvs0\" does not exist"
)

func KubeProxyMode(ctx context.Context, clusterInfo *cluster.Info, namespace string, imageOverrides []string, status reporter.Interface,
) error {
	status.Start("Checking Submariner support for the kube-proxy mode")
	defer status.End()

	if strings.EqualFold(clusterInfo.Submariner.Status.NetworkPlugin, cni.OVNKubernetes) {
		status.Success("Cluster is running with %q CNI which internally implements kube-proxy functionality", cni.OVNKubernetes)
		return nil
	}

	scheduling := pods.Scheduling{ScheduleOn: pods.GatewayNode, Networking: pods.HostNetworking}

	repositoryInfo, err := clusterInfo.GetImageRepositoryInfo(imageOverrides...)
	if err != nil {
		return status.Error(err, "Error determining repository information")
	}

	scheduled, err := pods.Schedule(ctx, &pods.Config{
		Name:                "query-iface-list",
		ClientSet:           clusterInfo.ClientProducer.ForKubernetes(),
		Scheduling:          scheduling,
		Namespace:           namespace,
		Command:             kubeProxyIPVSIfaceCommand,
		ImageRepositoryInfo: *repositoryInfo,
	})
	if err != nil {
		return status.Error(err, "Error spawning the network pod")
	}

	defer scheduled.Delete(ctx)

	if err = scheduled.AwaitCompletion(ctx); err != nil {
		return status.Error(err, "Error waiting for the network pod to finish its execution")
	}

	if !strings.Contains(scheduled.PodOutput, KubeProxyMissingInterface) && !strings.Contains(scheduled.PodOutput, KubeProxyNotEnabled) {
		return status.Error(errors.New("the cluster is deployed with kube-proxy ipvs mode which Submariner does not support"), "")
	}

	status.Success("The kube-proxy mode is supported")

	return nil
}
