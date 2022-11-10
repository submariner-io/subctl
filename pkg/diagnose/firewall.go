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
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	subv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/port"
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
	loadBalancerName = "submariner-gateway"
	encapsPortName   = "cable-encaps"
	nattPortName     = "natt-discovery"
)

const (
	singleNodeMessage = "Skipping this check as it's a single node cluster"
)

type FirewallOptions struct {
	ValidationTimeout uint
	VerboseOutput     bool
}

func spawnClientPodOnNonGatewayNode(client kubernetes.Interface, namespace, podCommand string,
	imageRepInfo *image.RepositoryInfo,
) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{ScheduleOn: pods.NonGatewayNode, Networking: pods.PodNetworking}

	return spawnPod(client, scheduling, "validate-client", namespace, podCommand, imageRepInfo)
}

func spawnClientPodOnNonGatewayNodeWithHostNet(client kubernetes.Interface, namespace, podCommand string,
	imageRepInfo *image.RepositoryInfo,
) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{ScheduleOn: pods.NonGatewayNode, Networking: pods.HostNetworking}
	return spawnPod(client, scheduling, "validate-client", namespace, podCommand, imageRepInfo)
}

func spawnPod(client kubernetes.Interface, scheduling pods.Scheduling, podName, namespace,
	podCommand string, imageRepInfo *image.RepositoryInfo,
) (*pods.Scheduled, error) {
	pod, err := pods.Schedule(&pods.Config{
		Name:                podName,
		ClientSet:           client,
		Scheduling:          scheduling,
		Namespace:           namespace,
		Command:             podCommand,
		ImageRepositoryInfo: *imageRepInfo,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error scheduling pod")
	}

	return pod, nil
}

func spawnSnifferPodOnNode(client kubernetes.Interface, nodeName, namespace, podCommand string,
	imageRepInfo *image.RepositoryInfo,
) (*pods.Scheduled, error) {
	scheduling := pods.Scheduling{
		ScheduleOn: pods.CustomNode, NodeName: nodeName,
		Networking: pods.HostNetworking,
	}

	return spawnPod(client, scheduling, "validate-sniffer", namespace, podCommand, imageRepInfo)
}

func getActiveGatewayNodeName(clusterInfo *cluster.Info, hostname string, imageRepInfo *image.RepositoryInfo,
	status reporter.Interface,
) (string, error) {
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
		sPod, err := spawnSnifferPodOnNode(clusterInfo.ClientProducer.ForKubernetes(), node.Name, constants.OperatorNamespace, "hostname",
			imageRepInfo)
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

func verifyConnectivity(localClusterInfo, remoteClusterInfo *cluster.Info, namespace string, options FirewallOptions,
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

	gwNodeName, err := getActiveGatewayNodeName(localClusterInfo, localEndpoint.Spec.Hostname,
		localClusterInfo.GetImageRepositoryInfo(), status)
	if err != nil {
		return err
	}

	destPort, err := getTargetPort(localClusterInfo.Submariner, localEndpoint, targetPort)
	if err != nil {
		return status.Error(err, "Could not determine the target port")
	}

	portFilter := fmt.Sprintf("dst port %d", destPort)

	lbNodePort, err := getLbNodePort(localClusterInfo, localEndpoint, targetPort)
	if err != nil {
		return status.Error(err, "Could not determine LB node port")
	}

	// When SM deployed using LB the encapsulated and nat discovery traffic received in some platforms on LB nodeport
	if lbNodePort != 0 {
		portFilter += fmt.Sprintf(" or dst port %d ", lbNodePort)
	}

	clientMessage := string(uuid.NewUUID())[0:8]
	// The following construct ensures that tcpdump will be stopped as soon as the message is seen, instead of waiting
	// for a timeout; but when the message isn't seen, it will be killed once the timeout expires
	podCommand := fmt.Sprintf(
		"(tcpdump --immediate-mode -ln -Q in -A -s 100 -i any udp and %s & pid=\"$!\"; (sleep %d; kill \"$pid\") &) | grep -m1 '%s'",
		portFilter, options.ValidationTimeout, clientMessage)

	sPod, err := spawnSnifferPodOnNode(localClusterInfo.ClientProducer.ForKubernetes(), gwNodeName, namespace, podCommand,
		localClusterInfo.GetImageRepositoryInfo())
	if err != nil {
		return status.Error(err, "Error spawning the sniffer pod on the Gateway node %q", gwNodeName)
	}

	defer sPod.Delete()

	gatewayPodIP, err := getGatewayIP(remoteClusterInfo, localClusterInfo.Submariner.Status.ClusterID, status)
	if err != nil {
		return status.Error(err, "Error retrieving the gateway IP of cluster %q", localClusterInfo.Name)
	}

	podCommand = fmt.Sprintf("for x in $(seq 1000); do echo %s; done | for i in $(seq 5);"+
		" do timeout 2 nc -n -p %s -u %s %d; done", clientMessage, clientSourcePort, gatewayPodIP, destPort)

	// Spawn the pod on the nonGateway node. If we spawn the pod on Gateway node, the tunnel process can
	// sometimes drop the udp traffic from client pod until the tunnels are properly setup.
	cPod, err := spawnClientPodOnNonGatewayNodeWithHostNet(remoteClusterInfo.ClientProducer.ForKubernetes(), namespace,
		podCommand, localClusterInfo.GetImageRepositoryInfo())
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

func getTargetPort(submariner *v1alpha1.Submariner, endpoint *subv1.Endpoint, tgtport TargetPort) (int32, error) {
	var targetPort int32
	var err error

	switch endpoint.Spec.Backend {
	case "libreswan", "wireguard", "vxlan":
		if tgtport == TunnelPort {
			targetPort, err = endpoint.Spec.GetBackendPort(subv1.UDPPortConfig, int32(submariner.Spec.CeIPSecNATTPort))
			if err != nil {
				return 0, fmt.Errorf("error reading tunnel port: %w", err)
			}
		} else if tgtport == NatDiscoveryPort {
			targetPort, err = endpoint.Spec.GetBackendPort(subv1.NATTDiscoveryPortConfig, port.NATTDiscovery)
			if err != nil {
				return 0, fmt.Errorf("error reading nat-discovery port: %w", err)
			}
		}

		return targetPort, nil
	default:
		return 0, fmt.Errorf("could not determine the target port for cable driver %q", endpoint.Spec.Backend)
	}
}

func getLbNodePort(clusterInfo *cluster.Info, endpoint *subv1.Endpoint, tgtport TargetPort) (int32, error) {
	usingLoadBalancer, _ := endpoint.Spec.GetBackendBool(subv1.UsingLoadBalancer, nil)
	if usingLoadBalancer == nil || !*usingLoadBalancer {
		return 0, nil
	}

	portName := encapsPortName
	if tgtport == NatDiscoveryPort {
		portName = nattPortName
	}

	svc, err := clusterInfo.ClientProducer.ForKubernetes().CoreV1().Services(endpoint.GetNamespace()).Get(
		context.TODO(), loadBalancerName, metav1.GetOptions{})
	if err == nil {
		for _, port := range svc.Spec.Ports {
			if port.Name == portName {
				return port.NodePort, nil
			}
		}
	} else {
		return 0, fmt.Errorf("error reading the details of LB service %s: %w", loadBalancerName, err)
	}

	return 0, fmt.Errorf("could not determine nodePort for port name %q of LB service %s", portName, loadBalancerName)
}
