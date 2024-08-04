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

var gatherFuncs = map[string]func(string, Info) bool{
	component.Connectivity:     gatherConnectivity,
	component.ServiceDiscovery: gatherDiscovery,
	component.Broker:           gatherBroker,
	component.Operator:         gatherOperator,
}

func Data(clusterInfo *cluster.Info, options Options) error {
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

	gatherDataByCluster(clusterInfo, options)

	fmt.Printf("Files are stored under directory %q\n", options.Directory)

	warnings := warningsBuf.String()
	if warnings != "" {
		fmt.Printf("\nEncountered following Kubernetes warnings while running:\n%s", warnings)
	}

	return nil
}

func gatherDataByCluster(clusterInfo *cluster.Info, options Options) {
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
			gatherFuncs[module](dataType, info)
			info.Status.End()
		}
	}

	gatherClusterSummary(&info)
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherConnectivity(dataType string, info Info) bool {
	if info.Submariner == nil {
		info.Status.Warning("The Submariner connectivity components are not installed")
		return true
	}

	switch dataType {
	case Logs:
		gatherGatewayPodLogs(&info)
		gatherRouteAgentPodLogs(&info)
		gatherMetricsProxyPodLogs(&info)
		gatherGlobalnetPodLogs(&info)
		gatherAddonPodLogs(&info)
	case Resources:
		gatherCNIResources(&info, info.Submariner.Status.NetworkPlugin)
		gatherCableDriverResources(&info, info.Submariner.Spec.CableDriver)
		gatherOVNResources(&info, info.Submariner.Status.NetworkPlugin)
		gatherEndpoints(&info, info.Submariner.Spec.Namespace)
		gatherClusters(&info, info.Submariner.Spec.Namespace)
		gatherGateways(&info, info.Submariner.Spec.Namespace)
		gatherClusterGlobalEgressIPs(&info)
		gatherGlobalEgressIPs(&info)
		gatherGlobalIngressIPs(&info)
	default:
		return false
	}

	return true
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherDiscovery(dataType string, info Info) bool {
	if info.ServiceDiscovery == nil {
		info.Status.Warning("The Submariner service discovery components are not installed")
		return true
	}

	switch dataType {
	case Logs:
		gatherServiceDiscoveryPodLogs(&info)
		gatherCoreDNSPodLogs(&info)
	case Resources:
		gatherServiceExports(&info, corev1.NamespaceAll)
		gatherServiceImports(&info, corev1.NamespaceAll)
		gatherEndpointSlices(&info, corev1.NamespaceAll)
		gatherConfigMapLighthouseDNS(&info, info.ServiceDiscovery.Namespace)
		gatherConfigMapCoreDNS(&info)
		gatherLabeledServices(&info, internalSvcLabel)
	default:
		return false
	}

	return true
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherBroker(dataType string, info Info) bool {
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
			err = info.ClientProducer.ForGeneral().Get(context.TODO(), controllerClient.ObjectKey{
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
		gatherEndpoints(&info, brokerNamespace)
		gatherClusters(&info, brokerNamespace)
		gatherEndpointSlices(&info, brokerNamespace)
		gatherServiceImports(&info, brokerNamespace)
	default:
		return false
	}

	return true
}

//nolint:gocritic // hugeParam: info - purposely passed by value.
func gatherOperator(dataType string, info Info) bool {
	switch dataType {
	case Logs:
		gatherSubmarinerOperatorPodLogs(&info)
	case Resources:
		gatherSubmariners(&info, info.OperatorNamespace())
		gatherServiceDiscoveries(&info, info.OperatorNamespace())
		gatherSubmarinerOperatorDeployment(&info, info.OperatorNamespace())
		gatherGatewayDaemonSet(&info, info.OperatorNamespace())
		gatherMetricsPodDaemonSet(&info, info.OperatorNamespace())
		gatherRouteAgentDaemonSet(&info, info.OperatorNamespace())
		gatherGlobalnetDaemonSet(&info, info.OperatorNamespace())
		gatherLighthouseAgentDeployment(&info, info.OperatorNamespace())
		gatherLighthouseCoreDNSDeployment(&info, info.OperatorNamespace())
		gatherGatewayLBService(&info, info.OperatorNamespace())
	default:
		return false
	}

	return true
}
