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

package show

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/show/table"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/images"
	"github.com/submariner-io/submariner-operator/pkg/names"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getOperatorVersion(clusterInfo *cluster.Info) ([]interface{}, error) {
	deployments := clusterInfo.LegacyClientProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace)

	operatorDeployment, err := deployments.Get(context.TODO(), names.OperatorComponent, v1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, errors.Wrap(err, "error retrieving Deployment")
	}

	version, repository := images.ParseOperatorImage(operatorDeployment.Spec.Template.Spec.Containers[0].Image)

	return []interface{}{names.OperatorComponent, repository, version}, nil
}

func getServiceDiscoveryVersion(clusterInfo *cluster.Info) ([]interface{}, error) {
	serviceDiscovery := &v1alpha1.ServiceDiscovery{}

	err := clusterInfo.ClientProducer.ForGeneral().Get(context.TODO(), controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      names.ServiceDiscoveryCrName,
	}, serviceDiscovery)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, errors.Wrap(err, "error retrieving Submariner resource")
	}

	return []interface{}{names.ServiceDiscoveryCrName, serviceDiscovery.Spec.Repository, serviceDiscovery.Spec.Version}, nil
}

func Versions(clusterInfo *cluster.Info, status reporter.Interface) bool {
	status.Start("Showing versions")

	printer := table.Printer{Columns: []table.Column{
		{Name: "COMPONENT"},
		{Name: "REPOSITORY"},
		{Name: "VERSION"},
	}}

	submariner := clusterInfo.Submariner
	if submariner != nil {
		printer.Add(names.SubmarinerCrName, submariner.Spec.Repository, submariner.Spec.Version)
	}

	operatorVersion, err := getOperatorVersion(clusterInfo)
	if err != nil {
		status.Failure("Unable to get the Operator version", err)
		status.End()

		return false
	}

	if operatorVersion != nil {
		printer.Add(operatorVersion...)
	}

	lighthouseVersion, err := getServiceDiscoveryVersion(clusterInfo)
	if err != nil {
		status.Failure("Unable to get the Service-Discovery version", err)
		status.End()

		return false
	}

	if lighthouseVersion != nil {
		printer.Add(lighthouseVersion...)
	}

	status.End()

	printer.Print()

	return true
}
