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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/utils/strings/slices"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Info struct {
	Name             string
	RestConfig       *rest.Config
	ClientProducer   client.Producer
	Submariner       *v1alpha1.Submariner
	ServiceDiscovery *v1alpha1.ServiceDiscovery
	nodeCount        int
}

func NewInfo(clusterName string, config *rest.Config) (*Info, error) {
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
	err = info.ClientProducer.ForGeneral().Get(context.TODO(), controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      names.SubmarinerCrName,
	}, submariner)

	if err == nil {
		info.Submariner = submariner
	} else if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return nil, errors.Wrap(err, "error retrieving Submariner")
	}

	serviceDiscovery := &v1alpha1.ServiceDiscovery{}
	err = info.ClientProducer.ForGeneral().Get(context.TODO(), controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      names.ServiceDiscoveryCrName,
	}, serviceDiscovery)

	if err == nil {
		info.ServiceDiscovery = serviceDiscovery
	} else if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return nil, errors.Wrap(err, "error retrieving ServiceDiscovery")
	}

	return info, nil
}

func (c *Info) GetGateways() ([]submarinerv1.Gateway, error) {
	gateways := &submarinerv1.GatewayList{}

	err := c.ClientProducer.ForGeneral().List(context.TODO(), gateways, controllerClient.InNamespace(constants.OperatorNamespace))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []submarinerv1.Gateway{}, nil
		}

		return nil, err //nolint:wrapcheck // error can't be wrapped.
	}

	return gateways.Items, nil
}

func (c *Info) HasSingleNode() (bool, error) {
	if c.nodeCount == -1 {
		nodes, err := c.ClientProducer.ForKubernetes().CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return false, errors.Wrap(err, "error listing Nodes")
		}

		c.nodeCount = len(nodes.Items)
	}

	return c.nodeCount == 1, nil
}

func (c *Info) GetLocalEndpoint() (*submarinerv1.Endpoint, error) {
	endpoints := &submarinerv1.EndpointList{}

	err := c.ClientProducer.ForGeneral().List(context.TODO(), endpoints, controllerClient.InNamespace(constants.OperatorNamespace))
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

func (c *Info) GetAnyRemoteEndpoint() (*submarinerv1.Endpoint, error) {
	endpoints := &submarinerv1.EndpointList{}

	err := c.ClientProducer.ForGeneral().List(context.TODO(), endpoints, controllerClient.InNamespace(constants.OperatorNamespace))
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

var validOverrides = []string{
	names.OperatorComponent,
	names.GatewayComponent,
	names.RouteAgentComponent,
	names.GlobalnetComponent,
	names.NetworkPluginSyncerComponent,
	names.ServiceDiscoveryComponent,
	names.LighthouseCoreDNSComponent,
	names.NettestComponent,
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
			return nil, fmt.Errorf("invalid override component %s provided. Please choose from %q", component, validOverrides)
		}

		imageOverrides[component] = imageURL
	}

	return imageOverrides, nil
}
