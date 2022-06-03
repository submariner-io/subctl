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

package diagnose

import (
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
)

func NatDiscoveryConfigAcrossClusters(localClusterInfo, remoteClusterInfo *cluster.Info, options FirewallOptions,
	status reporter.Interface,
) error {
	message := fmt.Sprintf("Checking if nat-discovery port is opened on the gateway node of cluster %q", localClusterInfo.Name)

	err := verifyConnectivity(localClusterInfo, remoteClusterInfo, options, status, NatDiscoveryPort, message)
	if err != nil {
		status.Failure("Could not determine if nat-discovery port is allowed in the cluster %q", localClusterInfo.Name)
	} else {
		status.Success("nat-discovery port is allowed in the cluster %q", localClusterInfo.Name)
	}

	return err
}
