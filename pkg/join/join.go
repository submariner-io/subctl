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
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/subctl/pkg/operator"
	"github.com/submariner-io/subctl/pkg/secret"
	"github.com/submariner-io/subctl/pkg/version"
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

//nolint:gocyclo // Cyclomatic complexity is mostly due to error checking so ignore.
func ClusterToBroker(ctx context.Context, brokerInfo *broker.Info, options *Options,
	clientProducer client.Producer, status reporter.Interface,
) error {
	err := checkRequirements(clientProducer.ForKubernetes(), options.IgnoreRequirements, brokerInfo, status)
	if err != nil {
		return err
	}

	err = isValidCustomCoreDNSConfig(options.CoreDNSCustomConfigMap)
	if err != nil {
		return status.Error(err, "error validating custom CoreDNS config")
	}

	imageOverrides, err := cluster.MergeImageOverrides(nil, options.ImageOverrideArr)
	if err != nil {
		return status.Error(err, "Error calculating image overrides")
	}

	status.Start("Gathering relevant information from Broker")
	defer status.End()

	brokerAdminConfig, err := brokerInfo.GetBrokerAdministratorConfig(ctx, !options.BrokerK8sSecure)
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

	clustersetConfig := clustersetip.Config{
		ClusterID:        options.ClusterID,
		ClustersetIPCIDR: options.ClustersetIPCIDR,
	}

	operatorNamespace := constants.OperatorNamespace

	err = ensureUniqueCluster(ctx, options.ClusterID, brokerClientProducer, brokerNamespace, clientProducer, operatorNamespace, status)
	if err != nil {
		return err
	}

	if options.GlobalnetEnabled {
		err = globalnet.AllocateAndUpdateGlobalCIDRConfigMap(ctx, brokerClientProducer.ForGeneral(), brokerNamespace, &netconfig,
			status)
		if err != nil {
			return errors.Wrap(err, "unable to determine the global CIDR")
		}
	}

	if brokerInfo.IsServiceDiscoveryEnabled() {
		enabled, err := clustersetip.AllocateCIDRFromConfigMap(ctx, brokerClientProducer.ForGeneral(), brokerNamespace,
			&clustersetConfig, status)
		if err != nil {
			return errors.Wrap(err, "unable to determine the clusterset IP CIDR")
		}

		if enabled {
			options.EnableClustersetIP = enabled
		}
	}

	status.Start("Deploying the Submariner operator")

	repositoryInfo := image.NewRepositoryInfo(options.Repository, options.ImageVersion, imageOverrides)

	err = operator.Ensure(ctx, status, clientProducer, operatorNamespace, repositoryInfo.GetOperatorImage(), options.OperatorDebug,
		&options.HTTPProxyConfig)
	if err != nil {
		return status.Error(err, "Error deploying the operator")
	}

	status.Start("Creating SA for cluster")

	brokerInfo.ClientToken, err = broker.CreateSAForCluster(ctx, brokerClientProducer.ForKubernetes(), options.ClusterID, brokerNamespace)
	if err != nil {
		return status.Error(err, "Error creating SA for cluster")
	}

	status.Start("Connecting to Broker")

	// We need to connect to the broker in all cases
	brokerSecret, err := secret.Ensure(ctx, clientProducer.ForKubernetes(), constants.OperatorNamespace, populateBrokerSecret(brokerInfo))
	if err != nil {
		return status.Error(err, "Error creating broker secret for cluster")
	}

	if brokerInfo.IsConnectivityEnabled() {
		status.Start("Deploying submariner")

		err := deploy.Submariner(ctx, clientProducer, submarinerOptionsFrom(options), brokerInfo, brokerSecret, netconfig,
			clustersetConfig, repositoryInfo, status)
		if err != nil {
			return status.Error(err, "Error deploying the Submariner resource")
		}

		status.Success("Submariner is up and running")
	} else if brokerInfo.IsServiceDiscoveryEnabled() {
		status.Start("Deploying service discovery only")

		err := deploy.ServiceDiscovery(ctx, clientProducer, serviceDiscoveryOptionsFrom(options), brokerInfo, brokerSecret,
			clustersetConfig, repositoryInfo, status)
		if err != nil {
			return status.Error(err, "Error deploying the ServiceDiscovery resource")
		}

		status.Success("Service discovery is up and running")
	}

	return nil
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
		ClustersetIPEnabled:           joinOptions.EnableClustersetIP,
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
		ClustersetIPEnabled:    joinOptions.EnableClustersetIP,
	}
}

func checkRequirements(kubeClient kubernetes.Interface, ignoreRequirements bool, brokerInfo *broker.Info, status reporter.Interface) error {
	_, failedRequirements, err := version.CheckRequirements(kubeClient, brokerInfo.ServiceDiscovery)

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
			Name: broker.LocalClientBrokerSecretName,
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

func ensureUniqueCluster(ctx context.Context, clusterID string, brokerProducer client.Producer, brokerNamespace string,
	localProducer client.Producer, operatorNamespace string, status reporter.Interface,
) error {
	err := localProducer.ForGeneral().Get(ctx, controllerClient.ObjectKey{
		Namespace: operatorNamespace,
		Name:      clusterID,
	}, &submarinerv1.Cluster{})

	if !resource.IsNotFoundErr(err) {
		return status.Error(err, "error retrieving local Cluster CR")
	}

	// This is a new installation - check if a Cluster CR exists on the broker. If so, it must be from another cluster.
	err = brokerProducer.ForGeneral().Get(ctx, controllerClient.ObjectKey{
		Namespace: brokerNamespace,
		Name:      clusterID,
	}, &submarinerv1.Cluster{})

	if resource.IsNotFoundErr(err) {
		return nil
	}

	if err != nil {
		return status.Error(err, "error retrieving broker Cluster CR")
	}

	status.Failure("Detected an existing joined cluster with the same ID %q on the broker. Cluster IDs must be unique across all clusters",
		clusterID)

	return goerrors.New("cluster ID not unique")
}
