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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/set"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct {
	Directory            string
	IncludeSensitiveData bool
	Modules              []string
	Types                []string
}

const (
	Logs      = "logs"
	Resources = "resources"
)

var AllModules = set.New(component.Connectivity, component.ServiceDiscovery, component.Broker, component.Operator)

var AllTypes = set.New(Logs, Resources)

var gatherFuncs = map[string]func(context.Context, string, Info) bool{
	component.Connectivity:     gatherConnectivity,
	component.ServiceDiscovery: gatherDiscovery,
	component.Broker:           gatherBroker,
	component.Operator:         gatherOperator,
}

func Data(ctx context.Context, clusterInfo *cluster.Info, options Options) error {
	var warningsBuf bytes.Buffer

	rest.SetDefaultWarningHandler(rest.NewWarningWriter(&warningsBuf, rest.WarningWriterOptions{
		Deduplicate: true,
	}))

	// concatenate the name of the cluster with the root gather directory
	options.Directory = filepath.Join(options.Directory, clusterInfo.Name)

	if _, err := os.Stat(options.Directory); os.IsNotExist(err) {
		err := os.MkdirAll(options.Directory, 0o700)
		if err != nil {
			exit.OnErrorWithMessage(err, fmt.Sprintf("Error creating directory %q", options.Directory))
		}
	}

	gatherDataByCluster(ctx, clusterInfo, options)

	fmt.Printf("Files are stored under directory %q\n", options.Directory)

	warnings := warningsBuf.String()
	if warnings != "" {
		fmt.Printf("\nEncountered following Kubernetes warnings while running:\n%s", warnings)
	}

	return nil
}

func gatherDataByCluster(ctx context.Context, clusterInfo *cluster.Info, options Options) {
	clusterName := clusterInfo.Name

	fmt.Printf("Gathering information from cluster %q\n", clusterName)

	info := Info{
		Info:                 *clusterInfo,
		ClusterName:          clusterName,
		DirName:              options.Directory,
		IncludeSensitiveData: options.IncludeSensitiveData,
		Summary:              &Summary{},
	}

	for _, module := range options.Modules {
		for _, dataType := range options.Types {
			info.Status = cli.NewReporter()
			info.Status.Start("Gathering %s %s", module, dataType)
			gatherFuncs[module](ctx, dataType, info)
			info.Status.End()
		}
	}

	gatherClusterSummary(ctx, &info)
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherConnectivity(ctx context.Context, dataType string, info Info) bool {
	if info.Submariner == nil {
		info.Status.Warning("The Submariner connectivity components are not installed")
		return true
	}

	switch dataType {
	case Logs:
		gatherGatewayPodLogs(ctx, &info)
		gatherRouteAgentPodLogs(ctx, &info)
		gatherMetricsProxyPodLogs(ctx, &info)
		gatherGlobalnetPodLogs(ctx, &info)
		gatherAddonPodLogs(ctx, &info)
	case Resources:
		gatherCNIResources(ctx, &info, info.Submariner.Status.NetworkPlugin)
		gatherCableDriverResources(ctx, &info, info.Submariner.Spec.CableDriver)
		gatherOVNResources(ctx, &info, info.Submariner.Status.NetworkPlugin)
		gatherEndpoints(ctx, &info, info.Submariner.Spec.Namespace)
		gatherClusters(ctx, &info, info.Submariner.Spec.Namespace)
		gatherGateways(ctx, &info, info.Submariner.Spec.Namespace)
		gatherRouteAgents(ctx, &info, info.Submariner.Spec.Namespace)
		gatherClusterGlobalEgressIPs(ctx, &info)
		gatherGlobalEgressIPs(ctx, &info)
		gatherGlobalIngressIPs(ctx, &info)
	default:
		return false
	}

	return true
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherDiscovery(ctx context.Context, dataType string, info Info) bool {
	if info.ServiceDiscovery == nil {
		info.Status.Warning("The Submariner service discovery components are not installed")
		return true
	}

	switch dataType {
	case Logs:
		gatherServiceDiscoveryPodLogs(ctx, &info)
		gatherCoreDNSPodLogs(ctx, &info)
	case Resources:
		gatherServiceExports(ctx, &info, corev1.NamespaceAll)
		gatherServiceImports(ctx, &info, corev1.NamespaceAll)
		gatherEndpointSlices(ctx, &info, corev1.NamespaceAll)
		gatherConfigMapLighthouseDNS(ctx, &info, info.ServiceDiscovery.Namespace)
		gatherConfigMapCoreDNS(ctx, &info)
		gatherLabeledServices(ctx, &info, internalSvcLabel)
	default:
		return false
	}

	return true
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherBroker(ctx context.Context, dataType string, info Info) bool {
	switch dataType {
	case Resources:
		brokerRestConfig, brokerNamespace, err := restconfig.ForBroker(info.Submariner, info.ServiceDiscovery)
		if err != nil {
			info.Status.Failure("Error getting the broker's rest config: %s", err)
			return true
		}

		if brokerRestConfig != nil {
			info.RestConfig = brokerRestConfig

			info.ClientProducer, err = client.NewProducerFromRestConfig(brokerRestConfig)
			if err != nil {
				info.Status.Failure("Error creating broker client Producer: %s", err)
				return true
			}
		} else {
			err = info.ClientProducer.ForGeneral().Get(ctx, controllerClient.ObjectKey{
				Namespace: constants.OperatorNamespace,
				Name:      brokercr.Name,
			}, &v1alpha1.Broker{})

			if resource.IsNotFoundErr(err) {
				return false
			}

			if err != nil {
				info.Status.Failure("Error getting the Broker resource: %s", err)
				return true
			}

			brokerNamespace = metav1.NamespaceAll
		}

		info.ClusterName = "broker"

		// The broker's ClusterRole used by member clusters only allows the below resources to be queried
		gatherEndpoints(ctx, &info, brokerNamespace)
		gatherClusters(ctx, &info, brokerNamespace)
		gatherEndpointSlices(ctx, &info, brokerNamespace)
		gatherServiceImports(ctx, &info, brokerNamespace)
	default:
		return false
	}

	return true
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherOperator(ctx context.Context, dataType string, info Info) bool {
	switch dataType {
	case Logs:
		gatherSubmarinerOperatorPodLogs(ctx, &info)
	case Resources:
		gatherSubmariners(ctx, &info, info.OperatorNamespace())
		gatherServiceDiscoveries(ctx, &info, info.OperatorNamespace())
		gatherSubmarinerOperatorDeployment(ctx, &info, info.OperatorNamespace())
		gatherGatewayDaemonSet(ctx, &info, info.OperatorNamespace())
		gatherMetricsPodDaemonSet(ctx, &info, info.OperatorNamespace())
		gatherRouteAgentDaemonSet(ctx, &info, info.OperatorNamespace())
		gatherGlobalnetDaemonSet(ctx, &info, info.OperatorNamespace())
		gatherLighthouseAgentDeployment(ctx, &info, info.OperatorNamespace())
		gatherLighthouseCoreDNSDeployment(ctx, &info, info.OperatorNamespace())
		gatherGatewayLBService(ctx, &info, info.OperatorNamespace())
	default:
		return false
	}

	return true
}
