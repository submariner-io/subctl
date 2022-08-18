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
	"github.com/submariner-io/submariner-operator/pkg/images"
	"github.com/submariner-io/submariner-operator/pkg/names"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func printDaemonSetVersions(clusterInfo *cluster.Info, printer *table.Printer, components ...string) error {
	daemonSets := clusterInfo.ClientProducer.ForKubernetes().AppsV1().DaemonSets(constants.OperatorNamespace)

	for _, component := range components {
		daemonSet, err := daemonSets.Get(context.TODO(), component, v1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

			return errors.Wrapf(err, "error retrieving %s DaemonSet", component)
		}

		// The name of the function is confusing, it just parses any image repo & version
		version, repository := images.ParseOperatorImage(daemonSet.Spec.Template.Spec.Containers[0].Image)
		printer.Add(component, repository, version)
	}

	return nil
}

func printDeploymentVersions(clusterInfo *cluster.Info, printer *table.Printer, components ...string) error {
	deployments := clusterInfo.ClientProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace)

	for _, component := range components {
		deployment, err := deployments.Get(context.TODO(), component, v1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

			return errors.Wrapf(err, "error retrieving %s Deployment", component)
		}

		version, repository := images.ParseOperatorImage(deployment.Spec.Template.Spec.Containers[0].Image)
		printer.Add(component, repository, version)
	}

	return nil
}

func fail(status reporter.Interface, err error) bool {
	status.Failure("Unable to determine versions: %v", err)
	status.End()

	return false
}

func Versions(clusterInfo *cluster.Info, status reporter.Interface) bool {
	status.Start("Showing versions")

	printer := table.Printer{Columns: []table.Column{
		{Name: "COMPONENT"},
		{Name: "REPOSITORY"},
		{Name: "VERSION"},
	}}

	err := printDaemonSetVersions(clusterInfo, &printer, names.GatewayComponent, names.RouteAgentComponent, names.GlobalnetComponent)
	if err != nil {
		return fail(status, err)
	}

	err = printDeploymentVersions(
		clusterInfo, &printer, names.OperatorComponent, names.ServiceDiscoveryComponent, names.LighthouseCoreDNSComponent)
	if err != nil {
		return fail(status, err)
	}

	status.End()
	printer.Print()

	return true
}
