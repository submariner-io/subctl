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

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
)

const (
	curlCmd              = "curl -I -m 10 --retry 6"
	gwCurlMetricsCommand = curlCmd + " submariner-gateway-metrics.submariner-operator.svc.cluster.local:8080/metrics"
	gnCurlMetricsCommand = curlCmd + " submariner-globalnet-metrics.submariner-operator.svc.cluster.local:8081/metrics"
)

func checkMetricsConfig(clusterInfo *cluster.Info, status reporter.Interface) bool {
	result := checkComponentMetrics(clusterInfo, status, "gateway", gwCurlMetricsCommand)

	if clusterInfo.Submariner.Spec.GlobalCIDR != "" {
		result = checkComponentMetrics(clusterInfo, status, "globalnet", gnCurlMetricsCommand) && result
	}

	return result
}

func checkComponentMetrics(clusterInfo *cluster.Info, status reporter.Interface, component, command string) bool {
	status.Start("Checking if %s metrics are accessible from non-gateway nodes", component)
	defer status.End()

	singleNode, err := clusterInfo.HasSingleNode()
	if err != nil {
		status.Failure(err.Error())
		return false
	}

	if singleNode {
		status.Success(singleNodeMessage)
		return true
	}

	cPod, err := spawnClientPodOnNonGatewayNode(clusterInfo.ClientProducer.ForKubernetes(),
		clusterInfo.Submariner.Namespace, command, clusterInfo.GetImageRepositoryInfo())
	if err != nil {
		status.Failure("Error spawning the client pod on non-Gateway node: %v", err)
		return false
	}

	defer cPod.Delete()

	if err = cPod.AwaitCompletion(); err != nil {
		status.Failure("Error waiting for the client pod to finish its execution: %v", err)
		return false
	}

	// Expected response: "HTTP/1.1 200 OK"
	if !strings.Contains(cPod.PodOutput, "200 OK") {
		status.Failure("Unable to access %s metrics service in submariner-operator "+
			"namespace %v", component, cPod.PodOutput)
		return false
	}

	status.Success("The %s metrics are accessible", component)

	return true
}
