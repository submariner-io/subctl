//go:build !non_deploy

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

package subctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/env"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/submariner-operator/pkg/ciliumcm"
)

func (c *JoinCommand) ensureCiliumClusterMeshPrerequisites(ctx context.Context, clusterInfo *cluster.Info,
	networkPlugin string, status reporter.Interface,
) error {
	err := deploy.EnsureCiliumClusterMeshPrerequisites(ctx, clusterInfo.ClientProducer.ForKubernetes(), "",
		networkPlugin, status)
	if err == nil {
		return nil
	}

	if !strings.EqualFold(networkPlugin, "cilium") {
		return err
	}

	ciliumNS, findErr := ciliumcm.FindUniqueCiliumConfigNamespace(ctx, clusterInfo.ClientProducer.ForKubernetes())
	if findErr != nil || ciliumNS == "" {
		return err
	}

	clusterID, clusterName, readErr := deploy.ReadCiliumClusterIdentity(ctx, clusterInfo.ClientProducer.ForKubernetes(), ciliumNS)
	if readErr != nil {
		return status.Error(readErr, "Error reading Cilium cluster identity")
	}

	if len(ciliumcm.LocalClusterIdentityFailures(clusterID, clusterName)) == 0 {
		return err
	}

	return c.configureCiliumClusterIdentity(ctx, clusterInfo.ClientProducer, ciliumNS,
		clusterID, clusterName, status)
}

func (c *JoinCommand) configureCiliumClusterIdentity(ctx context.Context, producer client.Producer, ciliumNS,
	currentID, currentName string, status reporter.Interface,
) error {
	problems := deploy.FormatCiliumIdentityProblems(currentID, currentName)

	if problems != "" {
		status.Warning("%s", problems)
	}

	fmt.Printf(`
Submariner needs a non-default Cilium cluster identity in ConfigMap %q
(namespace %q) for its ClusterMesh-shaped ipcache publisher.
cluster-id and cluster-name are read when Cilium agents start, so the
%s after the ConfigMap is updated.

`, ciliumcm.CiliumConfigMapName, ciliumNS, boldIfSmart("Cilium agent pods must be restarted"))

	configure := false
	err := survey.AskOne(NewInputPrompt(&survey.Confirm{
		Message: "Configure Cilium cluster-id and cluster-name now?",
		Default: true,
	}), &configure)
	if err != nil {
		if isNonInteractive(err) {
			return status.Error(errors.New(
				"Cilium cluster identity is invalid and subctl is running non-interactively; "+
					"set cilium-config cluster-id (1..254) and cluster-name, restart the Cilium agents, then re-run join"),
				"Cilium ClusterMesh prerequisites not met")
		}

		return status.Error(err, "Prompt failure")
	}

	if !configure {
		return status.Error(errors.New("Cilium cluster identity left unchanged"),
			"Cilium ClusterMesh prerequisites not met")
	}

	defaultName := currentName
	if defaultName == "" || defaultName == "default" || defaultName == ciliumcm.DefaultRemoteName {
		defaultName = c.flags.ClusterID
	}

	defaultID := ciliumcm.SuggestedLocalClusterID
	if idNum, parseErr := strconv.Atoi(currentID); parseErr == nil && idNum >= 1 && idNum <= 254 {
		defaultID = currentID
	}

	var clusterName string
	err = survey.AskOne(NewInputPrompt(&survey.Input{
		Message: "Cilium cluster-name",
		Default: defaultName,
	}), &clusterName, survey.WithValidator(func(val any) error {
		str, _ := val.(string)
		return ciliumcm.ValidateLocalClusterIdentity(ciliumcm.SuggestedLocalClusterID, strings.TrimSpace(str))
	}))
	if err != nil {
		return status.Error(err, "Prompt failure")
	}

	clusterName = strings.TrimSpace(clusterName)

	var clusterID string
	err = survey.AskOne(NewInputPrompt(&survey.Input{
		Message: "Cilium cluster-id (1..254; 255 is reserved for Submariner)",
		Default: defaultID,
	}), &clusterID, survey.WithValidator(func(val any) error {
		str, _ := val.(string)
		return ciliumcm.ValidateLocalClusterIdentity(strings.TrimSpace(str), clusterName)
	}))
	if err != nil {
		return status.Error(err, "Prompt failure")
	}

	clusterID = strings.TrimSpace(clusterID)

	restart := false
	err = survey.AskOne(NewInputPrompt(&survey.Confirm{
		Message: fmt.Sprintf(
			"Update ConfigMap %q and restart Cilium agent pods in namespace %q now?",
			ciliumcm.CiliumConfigMapName, ciliumNS),
		Default: true,
	}), &restart)
	if err != nil {
		return status.Error(err, "Prompt failure")
	}

	if !restart {
		return status.Error(errors.New("Cilium ConfigMap update canceled"),
			"Cilium ClusterMesh prerequisites not met")
	}

	status.Start("Updating Cilium ConfigMap %q in namespace %q", ciliumcm.CiliumConfigMapName, ciliumNS)

	kubeClient := producer.ForKubernetes()

	if err := deploy.SetCiliumClusterIdentity(ctx, kubeClient, ciliumNS, clusterID, clusterName); err != nil {
		return status.Error(err, "Error updating Cilium cluster identity")
	}

	status.Success("Updated cluster-id=%s cluster-name=%q", clusterID, clusterName)

	if err := deploy.RestartCiliumAgents(ctx, kubeClient, ciliumNS, status); err != nil {
		return err
	}

	return deploy.EnsureCiliumClusterMeshPrerequisites(ctx, kubeClient, ciliumNS, "cilium", status)
}

func boldIfSmart(s string) string {
	if env.IsSmartTerminal(os.Stdout) {
		return "\x1b[1m" + s + "\x1b[0m"
	}

	return s
}
