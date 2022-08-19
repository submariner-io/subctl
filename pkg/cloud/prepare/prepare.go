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

package prepare

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/submariner-operator/pkg/discovery/network"
	"github.com/submariner-io/submariner/pkg/cni"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getNetworkDetails(restCfgProducer *restconfig.Producer) (*network.ClusterNetwork, error) {
	k8sConfig, err := restCfgProducer.ForCluster()
	if err != nil {
		return nil, errors.Wrapf(err, "error creating the restConfig")
	}

	client, err := controllerClient.New(k8sConfig.Config, controllerClient.Options{})
	if err != nil {
		return nil, errors.Wrap(err, "error creating controller client")
	}

	networkDetails, err := network.Discover(client, constants.OperatorNamespace)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to discover network details")
	} else if networkDetails == nil {
		return nil, fmt.Errorf("no network details discovered")
	}

	return networkDetails, nil
}

func getPortConfig(restcfg *restconfig.Producer, ports *cloud.Ports, useNumericESP bool,
) ([]api.PortSpec, api.PrepareForSubmarinerInput, error) {
	gwPorts := []api.PortSpec{
		{Port: ports.Natt, Protocol: "udp"},
		{Port: ports.NatDiscovery, Protocol: "udp"},

		// ESP & AH protocols are used for private-ip to private-ip gateway communications.
		{Port: 0, Protocol: "esp"},
		{Port: 0, Protocol: "ah"},
	}

	if useNumericESP {
		for i, port := range gwPorts {
			switch port.Protocol {
			case "esp":
				gwPorts[i].Protocol = "50"
			case "ah":
				gwPorts[i].Protocol = "51"
			}
		}
	}

	input := api.PrepareForSubmarinerInput{}

	nwDetails, err := getNetworkDetails(restcfg)
	if err != nil {
		return gwPorts, input, errors.Wrapf(err, "failed to discover the network details in the cluster")
	}

	if nwDetails.NetworkPlugin != cni.OVNKubernetes {
		port := api.PortSpec{
			Port: ports.Vxlan, Protocol: "udp",
		}
		input.InternalPorts = append(input.InternalPorts, port)
	}

	return gwPorts, input, nil
}
