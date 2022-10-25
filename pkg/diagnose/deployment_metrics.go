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

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
	apierrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	curlCmd              = "curl -I -m 10 --retry 6"
	gwCurlMetricsCommand = curlCmd + " submariner-gateway-metrics.submariner-operator.svc.cluster.local:8080/metrics"
	gnCurlMetricsCommand = curlCmd + " submariner-globalnet-metrics.submariner-operator.svc.cluster.local:8081/metrics"
)

func checkMetricsConfig(clusterInfo *cluster.Info, status reporter.Interface) error {
	metricsErrors := []error{}
	if err := checkComponentMetrics(clusterInfo, status, "gateway", gwCurlMetricsCommand); err != nil {
		metricsErrors = append(metricsErrors, err)
	}

	if clusterInfo.Submariner.Spec.GlobalCIDR != "" {
		if err := checkComponentMetrics(clusterInfo, status, "globalnet", gnCurlMetricsCommand); err != nil {
			metricsErrors = append(metricsErrors, err)
		}
	}

	return apierrors.NewAggregate(metricsErrors)
}

func checkComponentMetrics(clusterInfo *cluster.Info, status reporter.Interface, component, command string) error {
	status.Start("Checking if %s metrics are accessible from non-gateway nodes", component)
	defer status.End()

	singleNode, err := clusterInfo.HasSingleNode()
	if err != nil {
		return status.Error(err, "Error determining whether the cluster has a single node")
	}

	if singleNode {
		status.Success(singleNodeMessage)
		return nil
	}

	cPod, err := spawnClientPodOnNonGatewayNode(clusterInfo.ClientProducer.ForKubernetes(),
		clusterInfo.Submariner.Namespace, command, clusterInfo.GetImageRepositoryInfo())
	if err != nil {
		return status.Error(err, "Error spawning the client pod on non-Gateway node")
	}

	defer cPod.Delete()

	if err = cPod.AwaitCompletion(); err != nil {
		return status.Error(err, "Error waiting for the client pod to finish its execution")
	}

	// Expected response: "HTTP/1.1 200 OK"
	if !strings.Contains(cPod.PodOutput, "200 OK") {
		return status.Error(errors.Errorf("Unexpected output %v", cPod.PodOutput),
			"Unable to access %s metrics service in submariner-operator namespace", component)
	}

	status.Success("The %s metrics are accessible", component)

	return nil
}
