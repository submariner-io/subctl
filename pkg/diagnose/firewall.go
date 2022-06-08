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
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/submariner/v1alpha1"
	subv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
)

type TargetPort int

const (
	TunnelPort TargetPort = iota
	NatDiscoveryPort
)

const (
	clientSourcePort = "9898"
)

const (
	singleNodeMessage = "Skipping this check as it's a single node cluster"
)

type FirewallOptions struct {
	ValidationTimeout uint
	VerboseOutput     bool
	PodNamespace      string
}

func spawnSnifferPodOnGatewayNode(client kubernetes.Interface, namespace, podCommand string) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{ScheduleOn: pods.GatewayNode, Networking: pods.HostNetworking}
	return spawnPod(client, scheduling, "validate-sniffer", namespace, podCommand)
}

func spawnClientPodOnNonGatewayNode(client kubernetes.Interface, namespace, podCommand string) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{ScheduleOn: pods.NonGatewayNode, Networking: pods.PodNetworking}
	return spawnPod(client, scheduling, "validate-client", namespace, podCommand)
}

func spawnPod(client kubernetes.Interface, scheduling pods.Scheduling, podName, namespace,
	podCommand string,
) (*pods.Scheduled, error) {
	pod, err := pods.Schedule(&pods.Config{
		Name:       podName,
		ClientSet:  client,
		Scheduling: scheduling,
		Namespace:  namespace,
		Command:    podCommand,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error scheduling pod")
	}

	return pod, nil
}

func spawnSnifferPodOnNode(client kubernetes.Interface, nodeName, namespace, podCommand string) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{
		ScheduleOn: pods.CustomNode, NodeName: nodeName,
		Networking: pods.HostNetworking,
	}

	return spawnPod(client, scheduling, "validate-sniffer", namespace, podCommand)
}

func getActiveGatewayNodeName(clusterInfo *cluster.Info, hostname string, status reporter.Interface) (string, error) {
	nodes, err := clusterInfo.ClientProducer.ForKubernetes().CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		LabelSelector: "submariner.io/gateway=true",
	})
	if err != nil {
		return "", status.Error(err, "Error obtaining the Gateway Nodes in cluster %q", clusterInfo.Name)
	}

	for i := range nodes.Items {
		node := &nodes.Items[i]
		if node.Name == hostname {
			return hostname, nil
		}

		// On some platforms, the nodeName does not match with the hostname.
		// Submariner Endpoint stores the hostname info in the endpoint and not the nodeName. So, we spawn a
		// tiny pod to read the hostname and return the corresponding node.
		sPod, err := spawnSnifferPodOnNode(clusterInfo.ClientProducer.ForKubernetes(), node.Name, "default", "hostname")
		if err != nil {
			return "", status.Error(err, "Error spawning the sniffer pod on the node %q: %v", node.Name)
		}

		err = sPod.AwaitCompletion()

		sPod.Delete()

		if err != nil {
			return "", status.Error(err, "Error waiting for the sniffer pod to finish its execution on node %q: %v", node.Name)
		}

		if sPod.PodOutput[:len(sPod.PodOutput)-1] == hostname {
			return node.Name, nil
		}
	}

	return "", status.Error(fmt.Errorf("could not find the active Gateway node %q in local cluster %q",
		hostname, clusterInfo.Name), "Error")
}

func getGatewayIP(clusterInfo *cluster.Info, localClusterID string, status reporter.Interface) (string, error) {
	gateways, err := clusterInfo.GetGateways()
	if err != nil {
		return "", status.Error(err, "Error retrieving gateways from cluster %q", clusterInfo.Name)
	}

	if len(gateways) == 0 {
		return "", status.Error(fmt.Errorf("there are no gateways detected on cluster %q", clusterInfo.Name), "Error")
	}

	for i := range gateways {
		gw := &gateways[i]
		if gw.Status.HAStatus != subv1.HAStatusActive {
			continue
		}

		for j := range gw.Status.Connections {
			conn := &gw.Status.Connections[j]
			if conn.Endpoint.ClusterID == localClusterID {
				if conn.UsingIP != "" {
					return conn.UsingIP, nil
				}

				if conn.Endpoint.NATEnabled {
					return conn.Endpoint.PublicIP, nil
				}

				return conn.Endpoint.PrivateIP, nil
			}
		}
	}

	return "", status.Error(fmt.Errorf("the gateway on cluster %q does not have an active connection to cluster %q",
		clusterInfo.Name, localClusterID), "Error")
}

func verifyConnectivity(localClusterInfo, remoteClusterInfo *cluster.Info, options FirewallOptions,
	status reporter.Interface, targetPort TargetPort, message string,
) error {
	mustHaveSubmariner(localClusterInfo)
	mustHaveSubmariner(remoteClusterInfo)

	status.Start(message)
	defer status.End()

	singleNode, err := remoteClusterInfo.HasSingleNode()
	if err != nil {
		return status.Error(err, "")
	}

	if singleNode {
		status.Success(singleNodeMessage)
		return nil
	}

	localEndpoint, err := localClusterInfo.GetLocalEndpoint()
	if err != nil {
		return status.Error(err, "Unable to obtain the local endpoint")
	}

	gwNodeName, err := getActiveGatewayNodeName(localClusterInfo, localEndpoint.Spec.Hostname, status)
	if err != nil {
		return err
	}

	destPort, err := getTargetPort(localClusterInfo.Submariner, localEndpoint, targetPort)
	if err != nil {
		return status.Error(err, "Could not determine the target port")
	}

	clientMessage := string(uuid.NewUUID())[0:8]
	podCommand := fmt.Sprintf("timeout %d tcpdump -ln -Q in -A -s 100 -i any udp and dst port %d | grep '%s'",
		options.ValidationTimeout, destPort, clientMessage)

	sPod, err := spawnSnifferPodOnNode(localClusterInfo.ClientProducer.ForKubernetes(), gwNodeName, options.PodNamespace, podCommand)
	if err != nil {
		return status.Error(err, "Error spawning the sniffer pod on the Gateway node %q", gwNodeName)
	}

	defer sPod.Delete()

	gatewayPodIP, err := getGatewayIP(remoteClusterInfo, localClusterInfo.Name, status)
	if err != nil {
		return status.Error(err, "Error retrieving the gateway IP of cluster %q", localClusterInfo.Name)
	}

	podCommand = fmt.Sprintf("for x in $(seq 1000); do echo %s; done | for i in $(seq 5);"+
		" do timeout 2 nc -n -p %s -u %s %d; done", clientMessage, clientSourcePort, gatewayPodIP, destPort)

	// Spawn the pod on the nonGateway node. If we spawn the pod on Gateway node, the tunnel process can
	// sometimes drop the udp traffic from client pod until the tunnels are properly setup.
	cPod, err := spawnClientPodOnNonGatewayNode(remoteClusterInfo.ClientProducer.ForKubernetes(), options.PodNamespace, podCommand)
	if err != nil {
		return status.Error(err, "Error spawning the client pod on non-Gateway node of cluster %q", remoteClusterInfo.Name)
	}

	defer cPod.Delete()

	if err = cPod.AwaitCompletion(); err != nil {
		return status.Error(err, "Error waiting for the client pod to finish its execution")
	}

	if err = sPod.AwaitCompletion(); err != nil {
		return status.Error(err, "Error waiting for the sniffer pod to finish its execution")
	}

	if options.VerboseOutput {
		status.Success("tcpdump output from sniffer pod on Gateway node")
		status.Success(sPod.PodOutput)
	}

	if !strings.Contains(sPod.PodOutput, clientMessage) {
		return status.Error(fmt.Errorf("the tcpdump output from the sniffer pod does not include the message"+
			" sent from client pod. Please check that your firewall configuration allows UDP/%d traffic"+
			" on the %q node", destPort, localEndpoint.Spec.Hostname), "Error")
	}

	return nil
}

func getTargetPort(submariner *v1alpha1.Submariner, endpoint *subv1.Endpoint, port TargetPort) (int32, error) {
	var targetPort int32
	var err error

	switch endpoint.Spec.Backend {
	case "libreswan", "wireguard", "vxlan":
		if port == TunnelPort {
			targetPort, err = endpoint.Spec.GetBackendPort(subv1.UDPPortConfig, int32(submariner.Spec.CeIPSecNATTPort))
			if err != nil {
				return 0, fmt.Errorf("error reading tunnel port: %w", err)
			}
		} else if port == NatDiscoveryPort {
			intValue, _ := strconv.ParseInt(subv1.DefaultNATTDiscoveryPort, 0, 32)
			targetPort, err = endpoint.Spec.GetBackendPort(subv1.NATTDiscoveryPortConfig, int32(intValue))
			if err != nil {
				return 0, fmt.Errorf("error reading nat-discovery port: %w", err)
			}
		}

		return targetPort, nil
	default:
		return 0, fmt.Errorf("could not determine the target port for cable driver %q", endpoint.Spec.Backend)
	}
}
