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

	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Info struct {
	Name           string
	RestConfig     *rest.Config
	ClientProducer client.Producer
	Submariner     *v1alpha1.Submariner
	nodeCount      int
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
		Namespace: constants.SubmarinerNamespace,
		Name:      names.SubmarinerCrName,
	}, submariner)

	if err == nil {
		info.Submariner = submariner
	} else if !apierrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "error retrieving Submariner")
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

		return nil, err // nolint:wrapcheck // error can't be wrapped.
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

func (c *Info) GetImageRepositoryInfo() *image.RepositoryInfo {
	info := &image.RepositoryInfo{}

	if c.Submariner != nil {
		info.Name = c.Submariner.Spec.Repository
		info.Version = c.Submariner.Spec.Version
		info.Overrides = c.Submariner.Spec.ImageOverrides
	}

	if info.Name == "" {
		info.Name = v1alpha1.DefaultRepo
	}

	if info.Version == "" {
		info.Version = v1alpha1.DefaultSubmarinerOperatorVersion
	}

	return info
}
