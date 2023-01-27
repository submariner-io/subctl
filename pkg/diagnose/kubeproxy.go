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
	"strings"

	"github.com/spf13/pflag"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/pkg/cluster"
)

const (
	kubeProxyIPVSIfaceCommand = "ip a s kube-ipvs0"
	missingInterface          = "ip: can't find device"
	notEnabled                = "Device \"kube-ipvs0\" does not exist"
)

var kubeProxyImageOverrides = []string{}

func AddKubeProxyImageOverrideFlag(flags *pflag.FlagSet) {
	flags.StringSliceVar(&firewallImageOverrides, "image-override", nil, "override component image")
}

func KubeProxyMode(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	status.Start("Checking Submariner support for the kube-proxy mode")
	defer status.End()

	scheduling := pods.Scheduling{ScheduleOn: pods.GatewayNode, Networking: pods.HostNetworking}

	repositoryInfo, err := clusterInfo.GetImageRepositoryInfo(kubeProxyImageOverrides...)
	if err != nil {
		return status.Error(err, "Error determining repository information")
	}

	podOutput, err := pods.ScheduleAndAwaitCompletion(&pods.Config{
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

	if !(strings.Contains(podOutput, missingInterface) || strings.Contains(podOutput, notEnabled)) {
		status.Failure("The cluster is deployed with kube-proxy ipvs mode which Submariner does not support")
		return nil
	}

	status.Success("The kube-proxy mode is supported")

	return nil
}
