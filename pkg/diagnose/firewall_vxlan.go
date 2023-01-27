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
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
)

const (
	tcpSniffVxLANCommand = "tcpdump -ln -c 3 -i vx-submariner tcp and port 8080 and 'tcp[tcpflags] == tcp-syn'"
)

func FirewallIntraVxLANConfig(clusterInfo *cluster.Info, namespace string, options FirewallOptions, status reporter.Interface) error {
	mustHaveSubmariner(clusterInfo)

	status.Start("Checking the firewall configuration to determine if intra-cluster VXLAN traffic is allowed")
	defer status.End()

	singleNode, err := clusterInfo.HasSingleNode()
	if err != nil {
		return status.Error(err, "Error determining whether the cluster has a single node")
	}

	if singleNode {
		status.Success(singleNodeMessage)
		return nil
	}

	tracker := reporter.NewTracker(status)

	checkFWConfig(clusterInfo, namespace, options, tracker)

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing the intra-VXLAN firewall configuration")
	}

	status.Success("The firewall configuration allows intra-cluster VXLAN traffic")

	return nil
}

func checkFWConfig(clusterInfo *cluster.Info, namespace string, options FirewallOptions, status reporter.Interface) {
	if clusterInfo.Submariner.Status.NetworkPlugin == "OVNKubernetes" {
		status.Success("This check is not necessary for the OVNKubernetes CNI plugin")
		return
	}

	remoteEndpoint, err := clusterInfo.GetAnyRemoteEndpoint()
	if err != nil {
		status.Failure("Unable to obtain a remote endpoint: %v", err)
		return
	}

	gwNodeName, err := getActiveGatewayNodeName(clusterInfo, status)
	if err != nil {
		status.Failure("Unable to obtain a gateway node: %v", err)
		return
	}

	podCommand := fmt.Sprintf("timeout %d %s", options.ValidationTimeout, tcpSniffVxLANCommand)

	repositoryInfo, err := clusterInfo.GetImageRepositoryInfo(firewallImageOverrides...)
	if err != nil {
		status.Failure("Error determining repository information: %v", err)
		return
	}

	sPod, err := spawnSnifferPodOnNode(clusterInfo.ClientProducer.ForKubernetes(), gwNodeName, namespace, podCommand, repositoryInfo)
	if err != nil {
		status.Failure("Error spawning the sniffer pod on the Gateway node: %v", err)
		return
	}

	defer sPod.Delete()

	remoteClusterIP := strings.Split(remoteEndpoint.Spec.Subnets[0], "/")[0]
	podCommand = fmt.Sprintf("nc -w %d %s 8080", options.ValidationTimeout/2, remoteClusterIP)

	cPod, err := spawnClientPodOnNonGatewayNode(clusterInfo.ClientProducer.ForKubernetes(), namespace, podCommand, repositoryInfo)
	if err != nil {
		status.Failure("Error spawning the client pod on non-Gateway node: %v", err)
		return
	}

	defer cPod.Delete()

	if err = cPod.AwaitCompletion(); err != nil {
		status.Failure("Error waiting for the client pod to finish its execution: %v", err)
		return
	}

	if err = sPod.AwaitCompletion(); err != nil {
		status.Failure("Error waiting for the sniffer pod to finish its execution: %v", err)
		return
	}

	if options.VerboseOutput {
		status.Success("tcpdump output from the sniffer pod on Gateway node:\n%s", sPod.PodOutput)
	}

	// Verify that tcpdump output (i.e, from snifferPod) contains the remoteClusterIP
	if !strings.Contains(sPod.PodOutput, remoteClusterIP) {
		status.Failure("The tcpdump output from the sniffer pod does not contain the expected remote"+
			" endpoint IP %s. Please check that your firewall configuration allows UDP/4800 traffic. Actual pod output: \n%s",
			remoteClusterIP, truncate(sPod.PodOutput))

		return
	}

	// Verify that tcpdump output (i.e, from snifferPod) contains the clientPod IPaddress
	if !strings.Contains(sPod.PodOutput, cPod.Pod.Status.PodIP) {
		status.Failure("The tcpdump output from the sniffer pod does not contain the client pod's IP."+
			" There seems to be some issue with the IPTable rules programmed on the %q node, Actual pod output: \n%s",
			cPod.Pod.Spec.NodeName, truncate(sPod.PodOutput))
	}
}
