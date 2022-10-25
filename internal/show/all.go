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
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"k8s.io/apimachinery/pkg/util/errors"
)

var showAllSubmarinerFunctions = []restconfig.PerContextFn{
	Connections,
	Endpoints,
	Gateways,
	Network,
	Versions,
}

func All(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	allErrors := []error{}

	allErrors = append(allErrors, Brokers(clusterInfo, namespace, status))

	fmt.Println()

	if clusterInfo.Submariner == nil {
		allErrors = append(allErrors, Versions(clusterInfo, namespace, status))

		fmt.Println()

		status.Warning(constants.SubmarinerNotInstalled)

		return errors.NewAggregate(allErrors)
	}

	for _, function := range showAllSubmarinerFunctions {
		allErrors = append(allErrors, function(clusterInfo, namespace, status))

		fmt.Println()
	}

	return errors.NewAggregate(allErrors)
}
