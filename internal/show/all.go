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
	"errors"
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
)

var showAllSubmarinerFunctions = []restconfig.PerContextFn{
	Connections,
	Endpoints,
	Gateways,
	Network,
	Versions,
}

func All(ctx context.Context, clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	allErrors := []error{}

	allErrors = append(allErrors, Brokers(ctx, clusterInfo, namespace, status))

	fmt.Println()

	if clusterInfo.Submariner == nil {
		allErrors = append(allErrors, Versions(ctx, clusterInfo, namespace, status))

		fmt.Println()

		status.Warning(constants.ConnectivityNotInstalled)

		return errors.Join(allErrors...)
	}

	for _, function := range showAllSubmarinerFunctions {
		allErrors = append(allErrors, function(ctx, clusterInfo, namespace, status))

		fmt.Println()
	}

	return errors.Join(allErrors...)
}
