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

package cluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/utils/strings/slices"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1b1 "sigs.k8s.io/mcs-api/pkg/apis/v1beta1"
)

type Info struct {
	Name             string
	RestConfig       *rest.Config
	ClientProducer   client.Producer
	Submariner       *v1alpha1.Submariner
	ServiceDiscovery *v1alpha1.ServiceDiscovery
	nodeCount        int
}

func NewInfo(ctx context.Context, clusterName string, config *rest.Config) (*Info, error) {
	info := &Info{
		Name:       clusterName,
		RestConfig: config,
		nodeCount:  -1,
	}

	var err error

	info.ClientProducer, err = client.NewProducerFromRestConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "error creating client producer")
	}

	submariner := &v1alpha1.Submariner{}

	err = info.ClientProducer.ForGeneral().Get(ctx, controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      opnames.SubmarinerCrName,
	}, submariner)
	if err == nil {
		info.Submariner = submariner
	} else if !resource.IsNotFoundErr(err) {
		return nil, errors.Wrap(err, "error retrieving Submariner")
	}

	serviceDiscovery := &v1alpha1.ServiceDiscovery{}

	err = info.ClientProducer.ForGeneral().Get(ctx, controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      opnames.ServiceDiscoveryCrName,
	}, serviceDiscovery)
	if err == nil {
		info.ServiceDiscovery = serviceDiscovery
	} else if !resource.IsNotFoundErr(err) {
		return nil, errors.Wrap(err, "error retrieving ServiceDiscovery")
	}

	_, err = info.GetGateways(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving Gateways")
	}

	return info, nil
}

func (c *Info) GetGateways(ctx context.Context) ([]submarinerv1.Gateway, error) {
	gateways := &submarinerv1.GatewayList{}

	err := c.ClientProducer.ForGeneral().List(ctx, gateways, controllerClient.InNamespace(constants.OperatorNamespace))
	if err != nil {
		if resource.IsNotFoundErr(err) {
			return []submarinerv1.Gateway{}, nil
		}

		return nil, err //nolint:wrapcheck // error can't be wrapped.
	}

	return gateways.Items, nil
}

func (c *Info) GetRouteAgents(ctx context.Context) ([]submarinerv1.RouteAgent, error) {
	routeAgents := &submarinerv1.RouteAgentList{}

	err := c.ClientProducer.ForGeneral().List(ctx, routeAgents, controllerClient.InNamespace(constants.OperatorNamespace))
	if err != nil {
		if resource.IsNotFoundErr(err) {
			return []submarinerv1.RouteAgent{}, nil
		}

		return nil, err //nolint:wrapcheck // error can't be wrapped.
	}

	return routeAgents.Items, nil
}

func (c *Info) HasSingleNode(ctx context.Context) (bool, error) {
	if c.nodeCount == -1 {
		nodes, err := c.ClientProducer.ForKubernetes().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, errors.Wrap(err, "error listing Nodes")
		}

		c.nodeCount = len(nodes.Items)
	}

	return c.nodeCount == 1, nil
}

func (c *Info) GetLocalEndpoint(ctx context.Context) (*submarinerv1.Endpoint, error) {
	endpoints := &submarinerv1.EndpointList{}

	err := c.ClientProducer.ForGeneral().List(ctx, endpoints, controllerClient.InNamespace(constants.OperatorNamespace))
	if err != nil {
		return nil, errors.Wrap(err, "error listing Endpoints")
	}

	for i := range endpoints.Items {
		if endpoints.Items[i].Spec.ClusterID == c.Submariner.Spec.ClusterID {
			return &endpoints.Items[i], nil
		}
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    submarinerv1.SchemeGroupVersion.Group,
		Resource: "endpoints",
	}, "local Endpoint")
}

func (c *Info) GetAnyRemoteEndpoint(ctx context.Context) (*submarinerv1.Endpoint, error) {
	endpoints := &submarinerv1.EndpointList{}

	err := c.ClientProducer.ForGeneral().List(ctx, endpoints, controllerClient.InNamespace(constants.OperatorNamespace))
	if err != nil {
		return nil, errors.Wrap(err, "error listing Endpoints")
	}

	for i := range endpoints.Items {
		if endpoints.Items[i].Spec.ClusterID != c.Submariner.Spec.ClusterID {
			return &endpoints.Items[i], nil
		}
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    submarinerv1.SchemeGroupVersion.Group,
		Resource: "endpoints",
	}, "remote Endpoint")
}

func (c *Info) GetImageRepositoryInfo(localImageOverrides ...string) (*image.RepositoryInfo, error) {
	if c.Submariner != nil {
		spec := c.Submariner.Spec

		imageOverrides, err := MergeImageOverrides(spec.ImageOverrides, localImageOverrides)
		if err != nil {
			return nil, err
		}

		return image.NewRepositoryInfo(spec.Repository, spec.Version, imageOverrides), nil
	}

	imageOverrides, err := MergeImageOverrides(make(map[string]string), localImageOverrides)
	if err != nil {
		return nil, err
	}

	return image.NewRepositoryInfo("", "", imageOverrides), nil
}

func (c *Info) OperatorNamespace() string {
	if c.Submariner != nil {
		return c.Submariner.Namespace
	}

	if c.ServiceDiscovery != nil {
		return c.ServiceDiscovery.Namespace
	}

	return constants.OperatorNamespace
}

func (c *Info) GetClusters(ctx context.Context, namespace string) ([]submarinerv1.Cluster, error) {
	clusters := &submarinerv1.ClusterList{}

	err := c.ClientProducer.ForGeneral().List(ctx, clusters, controllerClient.InNamespace(namespace))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving Clusters")
	}

	return clusters.Items, nil
}

var validOverrides = []string{
	names.OperatorComponent,
	names.GatewayComponent,
	names.RouteAgentComponent,
	names.GlobalnetComponent,
	names.ServiceDiscoveryComponent,
	names.LighthouseCoreDNSComponent,
	names.NettestComponent,
	names.MetricsProxyComponent,
}

func MergeImageOverrides(imageOverrides map[string]string, localImageOverrides []string) (map[string]string, error) {
	if imageOverrides == nil {
		imageOverrides = make(map[string]string, len(localImageOverrides))
	}

	for _, s := range localImageOverrides {
		component, imageURL, found := strings.Cut(s, "=")
		if !found {
			return nil, fmt.Errorf("invalid override %s provided. Please use `a=b` syntax", s)
		}

		if !slices.Contains(validOverrides, component) {
			return nil, fmt.Errorf("invalid image override component %q provided. Valid components are %q", component, validOverrides)
		}

		imageOverrides[component] = imageURL
	}

	return imageOverrides, nil
}

// NewBrokerRestConfig creates a REST config for connecting to the broker cluster.
// It prefers reading credentials from the broker Secret (more secure) but falls back
// to CR fields (BrokerK8sApiServerToken, BrokerK8sCA) for backwards compatibility.
// Returns the broker REST config and the broker namespace.
func (c *Info) NewBrokerRestConfig(ctx context.Context) (*rest.Config, string, error) {
	// Determine which CR to use (Submariner takes precedence over ServiceDiscovery)
	var brokerAPIServer, brokerNamespace, brokerSecretName, tokenField, caField string
	var gvResource schema.GroupVersionResource

	if c.Submariner != nil {
		brokerAPIServer = c.Submariner.Spec.BrokerK8sApiServer
		brokerNamespace = c.Submariner.Spec.BrokerK8sRemoteNamespace
		brokerSecretName = c.Submariner.Spec.BrokerK8sSecret
		tokenField = c.Submariner.Spec.BrokerK8sApiServerToken
		caField = c.Submariner.Spec.BrokerK8sCA
		gvResource = submarinerv1.SchemeGroupVersion.WithResource("clusters")
	} else if c.ServiceDiscovery != nil {
		brokerAPIServer = c.ServiceDiscovery.Spec.BrokerK8sApiServer
		brokerNamespace = c.ServiceDiscovery.Spec.BrokerK8sRemoteNamespace
		brokerSecretName = c.ServiceDiscovery.Spec.BrokerK8sSecret
		tokenField = c.ServiceDiscovery.Spec.BrokerK8sApiServerToken
		caField = c.ServiceDiscovery.Spec.BrokerK8sCA
		gvResource = gvr.FromMetaGroupVersion(mcsv1b1.GroupVersion, "serviceimports")
	} else {
		// Neither Submariner nor ServiceDiscovery CR found - return nil to allow caller to distinguish
		return nil, "", nil
	}

	var token, ca string
	var err error

	// Try to read broker credentials from Secret first (more secure)
	if brokerSecretName != "" {
		token, ca, err = c.readBrokerSecret(ctx, brokerSecretName)
		if err != nil {
			return nil, "", err
		}
	} else {
		// No Secret configured - use CR fields for backwards compatibility
		token = tokenField
		ca = caField
	}

	// Create REST config for broker using the credentials
	brokerRestConfig, _, err := resource.GetAuthorizedRestConfigFromData(
		brokerAPIServer,
		token,
		ca,
		&rest.TLSClientConfig{},
		gvResource,
		brokerNamespace)
	if err != nil {
		return nil, "", errors.Wrap(err, "error getting auth rest config for broker")
	}

	return brokerRestConfig, brokerNamespace, nil
}

// NewBrokerProducer creates a client.Producer for connecting to the broker cluster.
// It prefers reading credentials from the broker Secret (more secure) but falls back
// to CR fields (BrokerK8sApiServerToken, BrokerK8sCA) for backwards compatibility.
// Returns the broker client producer, broker REST config, and the broker namespace.
func (c *Info) NewBrokerProducer(ctx context.Context) (client.Producer, string, error) {
	brokerRestConfig, brokerNamespace, err := c.NewBrokerRestConfig(ctx)
	if err != nil {
		return nil, "", err
	}

	if brokerRestConfig == nil {
		// Neither Submariner nor ServiceDiscovery CR found
		return nil, "", nil
	}

	// Create client producer from the broker REST config
	brokerProducer, err := client.NewProducerFromRestConfig(brokerRestConfig)
	if err != nil {
		return nil, "", errors.Wrap(err, "error creating broker client producer")
	}

	return brokerProducer, brokerNamespace, nil
}

// readBrokerSecret reads the broker token and CA from a Secret using the cluster's ClientProducer.
func (c *Info) readBrokerSecret(ctx context.Context, secretName string) (string, string, error) {
	secret := &corev1.Secret{}

	err := c.ClientProducer.ForGeneral().Get(ctx, controllerClient.ObjectKey{
		Namespace: c.OperatorNamespace(),
		Name:      secretName,
	}, secret)
	if err != nil {
		return "", "", errors.Wrapf(err, "error reading broker secret %s/%s", c.OperatorNamespace(), secretName)
	}

	// Extract token data
	tokenData, ok := secret.Data["token"]
	if !ok || len(tokenData) == 0 {
		return "", "", errors.Errorf("broker secret %s/%s missing 'token' data", c.OperatorNamespace(), secretName)
	}

	// Extract ca.crt data
	caData, ok := secret.Data["ca.crt"]
	if !ok || len(caData) == 0 {
		return "", "", errors.Errorf("broker secret %s/%s missing 'ca.crt' data", c.OperatorNamespace(), secretName)
	}

	// Secret.Data is already decoded from base64 by Kubernetes API client
	// But GetAuthorizedRestConfigFromData expects CA to be base64-encoded
	// Token is used as-is (string)
	return string(tokenData), base64.StdEncoding.EncodeToString(caData), nil
}
