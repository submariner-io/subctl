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

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner/pkg/cni"
)

const (
	tcpSniffVxLANCommand = "tcpdump -ln -c 3 -i vx-submariner tcp and port 8080 and 'tcp[tcpflags] == tcp-syn'"
)

func FirewallIntraVxLANConfig(clusterInfo *cluster.Info, namespace string, options FirewallOptions, status reporter.Interface) error {
	mustHaveSubmariner(clusterInfo)

	status.Start("Checking that firewall configuration allows intra-cluster VXLAN traffic")
	defer status.End()

	return runIfSingleNode(clusterInfo, status, func() error {
		return checkFWConfig(clusterInfo, namespace, options, status)
	})
}

func checkFWConfig(clusterInfo *cluster.Info, namespace string, options FirewallOptions, status reporter.Interface) error {
	if clusterInfo.Submariner.Status.NetworkPlugin == cni.OVNKubernetes {
		return nil
	}

	remoteEndpoint, err := clusterInfo.GetAnyRemoteEndpoint()
	if err != nil {
		return status.Error(err, "Unable to obtain a remote endpoint")
	}

	gwNodeName, err := getActiveGatewayNodeName(clusterInfo, status)
	if err != nil {
		return err
	}

	podCommand := fmt.Sprintf("timeout %d %s", options.ValidationTimeout, tcpSniffVxLANCommand)

	repositoryInfo, err := clusterInfo.GetImageRepositoryInfo(options.ImageOverrides...)
	if err != nil {
		return status.Error(err, "Error determining repository information")
	}

	sPod, err := spawnSnifferPodOnNode(clusterInfo.ClientProducer.ForKubernetes(), gwNodeName, namespace, podCommand, repositoryInfo)
	if err != nil {
		return status.Error(err, "Error spawning the sniffer pod on the Gateway node %q", gwNodeName)
	}

	defer sPod.Delete()

	remoteClusterIP := strings.Split(remoteEndpoint.Spec.Subnets[0], "/")[0]
	podCommand = fmt.Sprintf("nc -w %d %s 8080", options.ValidationTimeout/2, remoteClusterIP)

	cPod, err := spawnClientPodOnNonGatewayNode(clusterInfo.ClientProducer.ForKubernetes(), namespace, podCommand, repositoryInfo)
	if err != nil {
		return status.Error(err, "Error spawning the client pod on non-Gateway node")
	}

	defer cPod.Delete()

	err = awaitPodCompletion(cPod, sPod, status)
	if err != nil {
		return err
	}

	if options.VerboseOutput {
		status.Success("tcpdump output from the sniffer pod on Gateway node:\n%s", sPod.PodOutput)
	}

	// Verify that tcpdump output (i.e, from snifferPod) contains the remoteClusterIP
	if !strings.Contains(sPod.PodOutput, remoteClusterIP) {
		return status.Error(fmt.Errorf("the tcpdump output from the sniffer pod does not contain the expected remote"+
			" endpoint IP %s. Please check that your firewall configuration allows UDP/4800 traffic. Actual pod output: \n%s",
			remoteClusterIP, truncate(sPod.PodOutput)), "")
	}

	// Verify that tcpdump output (i.e, from snifferPod) contains the clientPod IPaddress
	if !strings.Contains(sPod.PodOutput, cPod.Pod.Status.PodIP) {
		return status.Error(fmt.Errorf("the tcpdump output from the sniffer pod does not contain the client pod's IP."+
			" There seems to be some issue with the IPTable rules programmed on the %q node, Actual pod output: \n%s",
			cPod.Pod.Spec.NodeName, truncate(sPod.PodOutput)), "")
	}

	return nil
}
