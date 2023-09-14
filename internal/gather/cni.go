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

package gather

import (
	"context"

	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/submariner/pkg/cni"
	v1 "k8s.io/api/core/v1"
)

const (
	typeIPTables = "iptables"
	typeOvn      = "ovn"
	typeUnknown  = "unknown"
	libreswan    = "libreswan"
	vxlan        = "vxlan"
)

var systemCmds = map[string]string{
	"ip-a":              "ip -d a",
	"ip-l":              "ip -d l",
	"ip-routes":         "ip route show",
	"ip-rules":          "ip rule list",
	"ip-rules-table150": "ip rule show table 150",
	"sysctl-a":          "sysctl -a",
	"ipset-list":        "ipset list",
}

var ipGatewayCmds = map[string]string{
	"ip-routes-table150": "ip route show table 150",
}

var ipTablesCmds = map[string]string{
	"iptables":     "iptables -L -n -v --line-numbers",
	"iptables-nat": "iptables -L -n -v --line-numbers -t nat",
}

var libreswanCmds = map[string]string{
	"ip-xfrm-policy":      "ip xfrm policy",
	"ip-xfrm-state":       "ip xfrm state",
	"ipsec-status":        "ipsec status",
	"ipsec-trafficstatus": "ipsec --trafficstatus",
}

var vxlanCmds = map[string]string{
	"ip-routes-table100": "ip route show table 100",
}

const ovnNbctlShowCmd = "ovn-nbctl --no-leader-only show"

var ovnCmds = map[string]string{
	"ovn_nbctl_show":                     ovnNbctlShowCmd,
	"ovn_sbctl_show":                     "ovn-sbctl --no-leader-only show",
	"ovn_lr_ovn_cluster_router_policies": "ovn-nbctl --no-leader-only lr-policy-list ovn_cluster_router",
	"ovn_lr_ovn_cluster_router_routes":   "ovn-nbctl --no-leader-only lr-route-list ovn_cluster_router",
	"ovn_logical_routers":                "ovn-nbctl --no-leader-only list Logical_Router",
	"ovn_lrps":                           "ovn-nbctl --no-leader-only list Logical_Router_Port",
	"ovn_logical_switches":               "ovn-nbctl --no-leader-only list Logical_Switch",
	"ovn_lsps":                           "ovn-nbctl --no-leader-only list Logical_Switch_Port",
	"ovn_routes":                         "ovn-nbctl --no-leader-only list Logical_Router_Static_Route",
	"ovn_policies":                       "ovn-nbctl --no-leader-only list Logical_Router_Policy",
	"ovn_acls":                           "ovn-nbctl --no-leader-only list ACL",
	"ovn_lbgroups":                       "ovn-nbctl --no-leader-only list Load_Balancer_Group",
}

var networkPluginCNIType = map[string]string{
	cni.Generic:       typeIPTables,
	cni.Calico:        typeIPTables,
	cni.CanalFlannel:  typeIPTables,
	cni.Flannel:       typeIPTables,
	cni.KindNet:       typeIPTables,
	cni.OpenShiftSDN:  typeIPTables,
	cni.OVNKubernetes: typeOvn,
	cni.WeaveNet:      typeIPTables,
	"unknown":         typeUnknown,
}

func gatherCNIResources(info *Info, networkPlugin string) {
	logPodInfo(info, "CNI data", routeagentPodLabel, func(info *Info, pod *v1.Pod) {
		logSystemCmds(info, pod)
		switch networkPluginCNIType[networkPlugin] {
		case typeIPTables, typeOvn:
			logIPTablesCmds(info, pod)
		case typeUnknown:
			info.Status.Failure("Unsupported CNI Type")
		}
	})

	logCNIGatewayNodeResources(info)
}

func logCNIGatewayNodeResources(info *Info) {
	logPodInfo(info, "CNI data", gatewayPodLabel, logIPGatewayCmds)
}

func logSystemCmds(info *Info, pod *v1.Pod) {
	for name, cmd := range systemCmds {
		logCmdOutput(info, pod, cmd, name, false)
	}
}

func logIPGatewayCmds(info *Info, pod *v1.Pod) {
	for name, cmd := range ipGatewayCmds {
		logCmdOutput(info, pod, cmd, name, true)
	}
}

func logIPTablesCmds(info *Info, pod *v1.Pod) {
	for name, cmd := range ipTablesCmds {
		logCmdOutput(info, pod, cmd, name, false)
	}
}

func gatherOVNResources(info *Info, networkPlugin string) {
	if networkPluginCNIType[networkPlugin] != typeOvn {
		return
	}

	ovnKubePods := getOVNCmdsPod(info)
	for i := 0; i < len(ovnKubePods); i++ {
		info.Status.Success("Gathering OVN data from OVN kube pod %q", ovnKubePods[i].Name)

		for name, command := range ovnCmds {
			logCmdOutput(info, &ovnKubePods[i], command, name, false)
		}
	}

	gatherGatewayRoutes(info)
	gatherNonGatewayRoutes(info)
}

func getOVNCmdsPod(info *Info) []v1.Pod {
	/*
			We use three different labels for different OVNKubernetes deployments
		      * ovnMasterPodLabelOCP     - Non IC OpenShift
		      * ovnMasterPodLabelGeneric - Non IC upstreamovn-kubernetes
		      * ovnKubePodLabel          - IC Deployments, same for upstream and OpenShift
	*/
	ovnCmdPods, _ := findPods(info.ClientProducer.ForKubernetes(), ovnMasterPodLabelOCP)

	if len(ovnCmdPods.Items) > 0 {
		return []v1.Pod{ovnCmdPods.Items[0]}
	}

	ovnCmdPods, _ = findPods(info.ClientProducer.ForKubernetes(), ovnMasterPodLabelGeneric)
	if len(ovnCmdPods.Items) > 0 {
		return []v1.Pod{ovnCmdPods.Items[0]}
	}

	ovnCmdPods, err := findPods(info.ClientProducer.ForKubernetes(), ovnKubePodLabel)
	if len(ovnCmdPods.Items) > 0 {
		// TODO: Optimize this code to run OVN commands one node per zone instead of all nodes
		return ovnCmdPods.Items
	}

	if err != nil {
		info.Status.Failure("Failed to gather any OVN Kube pods: %v", err)
	} else {
		info.Status.Warning("No OVN kube pods found")
	}

	return nil
}

func gatherCableDriverResources(info *Info, cableDriver string) {
	logPodInfo(info, "cable driver data", gatewayPodLabel, func(info *Info, pod *v1.Pod) {
		if cableDriver == libreswan || cableDriver == "" { // If none specified, use libreswan as default
			logLibreswanCmds(info, pod)
		}
		if cableDriver == vxlan {
			logVxlanCmds(info, pod)
		}
	})
}

func logLibreswanCmds(info *Info, pod *v1.Pod) {
	for name, cmd := range libreswanCmds {
		logCmdOutput(info, pod, cmd, name, true)
	}
}

func logVxlanCmds(info *Info, pod *v1.Pod) {
	for name, cmd := range vxlanCmds {
		logCmdOutput(info, pod, cmd, name, true)
	}
}

//nolint:wrapcheck // No need to wrap errors here.
func execCmdInBash(info *Info, pod *v1.Pod, cmd string) (string, string, error) {
	execOptions := pods.ExecOptionsFromPod(pod)
	execConfig := pods.ExecConfig{
		RestConfig: info.RestConfig,
		ClientSet:  info.ClientProducer.ForKubernetes(),
	}

	execOptions.Command = []string{"/bin/bash", "-c", cmd}

	return pods.ExecWithOptions(context.TODO(), execConfig, &execOptions)
}

func logCmdOutput(info *Info, pod *v1.Pod, cmd, cmdName string, ignoreError bool) {
	stdOut, _, err := execCmdInBash(info, pod, cmd)
	if err != nil && !ignoreError {
		info.Status.Failure("Error running %q on pod %q: %v", cmd, pod.Name, err)

		return
	}

	if stdOut != "" {
		// the first line contains the executed command
		stdOut = cmd + "\n" + stdOut

		fileName, err := writeLogToFile(stdOut, pod.Spec.NodeName+"_"+cmdName, info, ".log")
		if err != nil {
			info.Status.Failure("Error writing output from command %q on pod %q: %v", cmd, pod.Name, err)
		}

		info.Summary.Resources = append(info.Summary.Resources, ResourceInfo{
			Namespace: pod.Namespace,
			Name:      pod.Spec.NodeName,
			FileName:  fileName,
			Type:      cmdName,
		})
	}
}
