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

package join

import (
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/subctl/pkg/operator"
	"github.com/submariner-io/subctl/pkg/secret"
	"github.com/submariner-io/subctl/pkg/version"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"github.com/submariner-io/submariner-operator/pkg/names"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/strings/slices"
)

var validOverrides = []string{
	names.OperatorComponent,
	names.GatewayComponent,
	names.RouteAgentComponent,
	names.GlobalnetComponent,
	names.NetworkPluginSyncerComponent,
	names.ServiceDiscoveryComponent,
	names.LighthouseCoreDNSComponent,
}

func ClusterToBroker(brokerInfo *broker.Info, options *Options, clientProducer client.Producer, status reporter.Interface,
) error {
	err := checkRequirements(clientProducer.ForKubernetes(), options.IgnoreRequirements, status)
	if err != nil {
		return err
	}

	err = isValidCustomCoreDNSConfig(options.CoreDNSCustomConfigMap)
	if err != nil {
		return status.Error(err, "error validating custom CoreDNS config")
	}

	imageOverrides, err := overridesArrToMap(options.ImageOverrideArr)
	if err != nil {
		return status.Error(err, "Error calculating image overrides")
	}

	status.Start("Gathering relevant information from Broker")
	defer status.End()

	brokerAdminConfig, err := brokerInfo.GetBrokerAdministratorConfig(!options.BrokerK8sSecure)
	if err != nil {
		return status.Error(err, "Error retrieving broker admin config")
	}

	brokerClientProducer, err := client.NewProducerFromRestConfig(brokerAdminConfig)
	if err != nil {
		return status.Error(err, "Error creating broker client producer")
	}

	brokerNamespace := string(brokerInfo.ClientToken.Data["namespace"])
	netconfig := globalnet.Config{
		ClusterID:   options.ClusterID,
		GlobalCIDR:  options.GlobalnetCIDR,
		ClusterSize: options.GlobalnetClusterSize,
	}

	if options.GlobalnetEnabled {
		err = globalnet.AllocateAndUpdateGlobalCIDRConfigMap(brokerClientProducer.ForGeneral(), brokerNamespace, &netconfig, status)
		if err != nil {
			return errors.Wrap(err, "unable to determine the global CIDR")
		}
	}

	status.Start("Deploying the Submariner operator")

	repositoryInfo := image.NewRepositoryInfo(options.Repository, options.ImageVersion, imageOverrides)

	err = operator.Ensure(status, clientProducer, constants.OperatorNamespace, repositoryInfo.GetOperatorImage(), options.OperatorDebug)
	if err != nil {
		return status.Error(err, "Error deploying the operator")
	}

	status.Start("Creating SA for cluster")

	brokerInfo.ClientToken, err = broker.CreateSAForCluster(brokerClientProducer.ForKubernetes(), options.ClusterID, brokerNamespace)
	if err != nil {
		return status.Error(err, "Error creating SA for cluster")
	}

	status.Start("Connecting to Broker")

	// We need to connect to the broker in all cases
	brokerSecret, err := secret.Ensure(clientProducer.ForKubernetes(), constants.OperatorNamespace, populateBrokerSecret(brokerInfo))
	if err != nil {
		return status.Error(err, "Error creating broker secret for cluster")
	}

	if brokerInfo.IsConnectivityEnabled() {
		status.Start("Deploying submariner")

		err := deploy.Submariner(clientProducer, submarinerOptionsFrom(options), brokerInfo, brokerSecret, netconfig,
			repositoryInfo, status)
		if err != nil {
			return status.Error(err, "Error deploying the Submariner resource")
		}

		status.Success("Submariner is up and running")
	} else if brokerInfo.IsServiceDiscoveryEnabled() {
		status.Start("Deploying service discovery only")

		err := deploy.ServiceDiscovery(clientProducer, serviceDiscoveryOptionsFrom(options), brokerInfo, brokerSecret,
			repositoryInfo, status)
		if err != nil {
			return status.Error(err, "Error deploying the ServiceDiscovery resource")
		}

		status.Success("Service discovery is up and running")
	}

	return nil
}

func overridesArrToMap(imageOverrideArr []string) (map[string]string, error) {
	imageOverrides := make(map[string]string)

	for _, s := range imageOverrideArr {
		component, imageURL, found := strings.Cut(s, "=")
		if !found {
			return nil, fmt.Errorf("invalid override %s provided. Please use `a=b` syntax", s)
		}

		if !slices.Contains(validOverrides, component) {
			return nil, fmt.Errorf("invalid override component %s provided. Please choose from %q", component, validOverrides)
		}

		imageOverrides[component] = imageURL
	}

	return imageOverrides, nil
}

func submarinerOptionsFrom(joinOptions *Options) *deploy.SubmarinerOptions {
	return &deploy.SubmarinerOptions{
		PreferredServer:               joinOptions.PreferredServer,
		ForceUDPEncaps:                joinOptions.ForceUDPEncaps,
		NATTraversal:                  joinOptions.NATTraversal,
		IPSecDebug:                    joinOptions.IPSecDebug,
		SubmarinerDebug:               joinOptions.SubmarinerDebug,
		AirGappedDeployment:           joinOptions.AirGappedDeployment,
		LoadBalancerEnabled:           joinOptions.LoadBalancerEnabled,
		HealthCheckEnabled:            joinOptions.HealthCheckEnabled,
		NATTPort:                      joinOptions.NATTPort,
		HealthCheckInterval:           joinOptions.HealthCheckInterval,
		HealthCheckMaxPacketLossCount: joinOptions.HealthCheckMaxPacketLossCount,
		ClusterID:                     joinOptions.ClusterID,
		CableDriver:                   joinOptions.CableDriver,
		CoreDNSCustomConfigMap:        joinOptions.CoreDNSCustomConfigMap,
		Repository:                    joinOptions.Repository,
		ImageVersion:                  joinOptions.ImageVersion,
		CustomDomains:                 joinOptions.CustomDomains,
		ServiceCIDR:                   joinOptions.ServiceCIDR,
		ClusterCIDR:                   joinOptions.ClusterCIDR,
		BrokerK8sInsecure:             !joinOptions.BrokerK8sSecure,
	}
}

func serviceDiscoveryOptionsFrom(joinOptions *Options) *deploy.ServiceDiscoveryOptions {
	return &deploy.ServiceDiscoveryOptions{
		SubmarinerDebug:        joinOptions.SubmarinerDebug,
		ClusterID:              joinOptions.ClusterID,
		CoreDNSCustomConfigMap: joinOptions.CoreDNSCustomConfigMap,
		Repository:             joinOptions.Repository,
		ImageVersion:           joinOptions.ImageVersion,
		CustomDomains:          joinOptions.CustomDomains,
		BrokerK8sInsecure:      !joinOptions.BrokerK8sSecure,
	}
}

func checkRequirements(kubeClient kubernetes.Interface, ignoreRequirements bool, status reporter.Interface) error {
	_, failedRequirements, err := version.CheckRequirements(kubeClient)

	if len(failedRequirements) > 0 {
		msg := "The target cluster fails to meet Submariner's version requirements:\n"
		for i := range failedRequirements {
			msg += fmt.Sprintf("* %s\n", failedRequirements[i])
		}

		if !ignoreRequirements {
			status.Failure(msg)

			return goerrors.New("version requirements not met")
		}

		status.Warning(msg)
	}

	return status.Error(err, "unable to check version requirements")
}

func populateBrokerSecret(brokerInfo *broker.Info) *v1.Secret {
	// We need to copy the broker token secret as an opaque secret to store it in the connecting cluster
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "broker-secret-",
		},
		Type: v1.SecretTypeOpaque,
		Data: brokerInfo.ClientToken.Data,
	}
}

func isValidCustomCoreDNSConfig(corednsCustomConfigMap string) error {
	if corednsCustomConfigMap != "" && strings.Count(corednsCustomConfigMap, "/") > 1 {
		return fmt.Errorf("coredns-custom-configmap should be in <namespace>/<name> format, namespace is optional")
	}

	return nil
}
