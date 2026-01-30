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
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/command"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/subctl/pkg/operator"
	"github.com/submariner-io/subctl/pkg/secret"
	"github.com/submariner-io/subctl/pkg/version"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var LatestReleaseURL = "https://api.github.com/repos/submariner-io/releases/releases/latest"

type upgradeCommand struct {
	subctlVersion      string
	operatorVersion    string
	submarinerVersion  string
	restConfigProducer *restconfig.Producer
}

func NewUpgradeCmd() *cobra.Command {
	upgradeCmd := &upgradeCommand{
		restConfigProducer: restconfig.NewProducer(),
	}

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades Submariner",
		Run:   upgradeCmd.upgrade,
	}

	cmd.Flags().StringVar(&upgradeCmd.subctlVersion, "to-version", "", "the version of subctl and Submariner to which to upgrade")
	cmd.Flags().StringVar(&upgradeCmd.operatorVersion, "to-operator-version", "", "the version of the operator to which to upgrade")
	_ = cmd.Flags().MarkHidden("to-operator-version")
	cmd.Flags().StringVar(&upgradeCmd.submarinerVersion, "to-submariner-version", "", "the version of Submariner to which to upgrade")
	_ = cmd.Flags().MarkHidden("to-submariner-version")
	upgradeCmd.restConfigProducer.SetupFlags(cmd.Flags())
	addHTTPProxyFlags(cmd.Flags(), &httpProxyConfig)

	return cmd
}

func init() {
	rootCmd.AddCommand(NewUpgradeCmd())
}

func (c *upgradeCommand) upgrade(cmd *cobra.Command, _ []string) {
	status := NewReporter()

	// Step 1: upgrade subctl to match the requested version
	commandPath, err := c.upgradeSubctl(status)
	exit.OnError(err)

	if commandPath != "" {
		// Step 2a: subctl was upgraded, so run it instead of continuing
		cmd := &exec.Cmd{
			Path:   commandPath,
			Args:   os.Args,
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
		// exit.OnError outputs the version of subctl, which ends up being confusing here
		if err := command.New(cmd).Run(); err != nil {
			os.Exit(1)
		}
	} else {
		// Step 2b: this subctl is already the requested version, run it
		exit.OnError(c.restConfigProducer.RunOnAllContexts(func(info *cluster.Info, namespace string, status reporter.Interface) error {
			return c.upgradeSubmariner(cmd.Context(), info, namespace, status)
		}, status))
	}
}

// upgradeSubctl upgrades the local copy of subctl, if necessary.
// Returns the path to the upgraded subctl if subctl was upgraded, nil if it wasn't.
func (c *upgradeCommand) upgradeSubctl(status reporter.Interface) (string, error) {
	// Default to downloading the latest version
	targetVersionString := "latest"

	// If the user hasn't specified a version, try to find the latest release on GitHub
	if c.subctlVersion == "" {
		tag, err := retrieveLatestReleaseTag()
		if err != nil {
			status.Warning("Couldn't retrieve the latest release tag, forcing a download: %v", err)
		} else {
			c.subctlVersion = tag
		}
	}

	if c.subctlVersion == version.Version {
		// Already running the right version
		return "", nil
	}

	if c.subctlVersion != "" {
		c.subctlVersion = strings.TrimPrefix(c.subctlVersion, "v")

		toVersion, err := semver.NewVersion(c.subctlVersion)
		if toVersion == nil {
			return "", status.Error(err, "Invalid target version")
		}

		// semver needs a dotted triplet, which is at least five characters;
		// on development or unknown versions, assume we need to upgrade
		if len(version.Version) >= 5 && !strings.HasPrefix(version.Version, "devel") && !strings.HasPrefix(version.Version, "release") {
			currentVersion, err := semver.NewVersion(strings.TrimPrefix(version.Version, "v"))
			if currentVersion == nil {
				return "", status.Error(err, "Error parsing current subctl version")
			}

			if toVersion.LessThan(*currentVersion) || toVersion.Equal(*currentVersion) {
				return "", nil
			}
		}

		targetVersionString = "v" + toVersion.String()
	}

	status.Start("Upgrading subctl from %s to %s, replacing %s", version.Version, targetVersionString, os.Args[0])

	url := "https://get.submariner.io"

	absolutePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return "", status.Error(err, "Error determining the installation path")
	}

	_, err = command.New(exec.Command( //nolint:gosec // The user-controlled variables are sanitised above
		"sh", "-c", "curl "+url+" | VERSION="+targetVersionString+" DESTDIR="+filepath.Dir(absolutePath)+" bash")).CombinedOutput()
	if err != nil {
		return "", status.Error(err, "Error upgrading subctl")
	}

	status.End()

	return absolutePath, nil
}

func retrieveLatestReleaseTag() (string, error) {
	// Retrieve the latest release tag from https://api.github.com/repos/submariner-io/releases/releases/latest
	// (the duplicate "releases" portion is normal, it points to the releases of the Submariner releases project)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	r, err := httpClient.Get(LatestReleaseURL)
	if err != nil {
		return "", fmt.Errorf("error accessing GitHub: %w", err)
	}
	defer r.Body.Close()

	response, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("error reading from GitHub: %w", err)
	}

	var unstructured map[string]any

	err = json.Unmarshal(response, &unstructured)
	if err != nil {
		return "", fmt.Errorf("error unmarshaling the JSON response from GitHub: %w", err)
	}

	tagName, ok := unstructured["tag_name"]
	if ok {
		return tagName.(string), nil
	}

	return "", goerrors.New("no tag name found in the latest release data")
}

func (c *upgradeCommand) upgradeSubmariner(ctx context.Context, clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	// We only expect users to specify a subctl version, if any ("--to-version"). In such scenarios,
	// the versions are expected to align, so subctl vX installs the operator image tagged with vX,
	// and that operator defaults to the appropriate Submariner version.
	// Other versions can be set for debugging purposes (to test installation with development versions,
	// before tags are aligned).
	// If the operator version isn't specified, it should match the version of subctl.
	// If the Submariner version isn't specified, it should be left blank so that the operator uses
	// its defaults.
	if c.operatorVersion == "" {
		c.operatorVersion = c.subctlVersion
	}

	// Upgrade Broker if installed; role updates are part of Broker redeploy
	brokerUpgraded, err := c.upgradeBroker(ctx, clusterInfo, status)
	if err != nil {
		return err
	}

	var repository string
	var debug bool
	var imageOverride map[string]string

	if clusterInfo.Submariner != nil {
		repository = clusterInfo.Submariner.Spec.Repository
		imageOverride = clusterInfo.Submariner.Spec.ImageOverrides
		debug = clusterInfo.Submariner.Spec.Debug
	} else if clusterInfo.ServiceDiscovery != nil {
		repository = clusterInfo.ServiceDiscovery.Spec.Repository
		imageOverride = clusterInfo.ServiceDiscovery.Spec.ImageOverrides
		debug = clusterInfo.ServiceDiscovery.Spec.Debug
	} else {
		// Nothing further to do
		return nil
	}

	// If a Broker was upgraded in this context, the Operator has already been upgraded
	if !brokerUpgraded {
		// Upgrade Operator if deployed
		if err := c.upgradeOperator(ctx, clusterInfo, repository, debug, imageOverride, status); err != nil {
			return err
		}
	}

	// We want to show the user a version; use the most specific one
	logVersion := c.subctlVersion
	if c.operatorVersion != "" {
		logVersion = c.operatorVersion
	}

	if c.submarinerVersion != "" {
		logVersion = c.submarinerVersion
	}

	// Upgrade Submariner
	if err := c.upgradeConnectivity(ctx, clusterInfo, logVersion, status); err != nil {
		return err
	}

	// Upgrade Service discovery
	return c.upgradeServiceDiscovery(ctx, clusterInfo, logVersion, status)
}

func (c *upgradeCommand) upgradeBroker(ctx context.Context, clusterInfo *cluster.Info, status reporter.Interface) (bool, error) {
	status.Start("Checking if the Broker is installed")
	defer status.End()

	brokerObj, brokerFound, err := getBroker(ctx, clusterInfo.RestConfig, constants.DefaultBrokerNamespace)
	if err != nil {
		return false, status.Error(err, "Error checking for the Broker")
	}

	if !brokerFound {
		return false, nil
	}

	status.Start("Upgrading the Broker to %s", c.operatorVersion)
	options := &deploy.BrokerOptions{
		ImageVersion:    c.operatorVersion,
		BrokerNamespace: brokerObj.Namespace,
		BrokerSpec:      brokerObj.Spec,
		HTTPProxyConfig: httpProxyConfig,
	}

	err = deploy.Deploy(ctx, options, status, clusterInfo.ClientProducer)

	return err == nil, status.Error(err, "Error upgrading the Broker")
}

func migrateBrokerSecret(ctx context.Context, kubeClient kubernetes.Interface, fromSecretName string, status reporter.Interface,
) (string, error) {
	if !strings.HasPrefix(fromSecretName, "broker-secret-") {
		return fromSecretName, nil
	}

	ns := constants.OperatorNamespace

	_, err := kubeClient.CoreV1().Secrets(ns).Get(ctx, broker.LocalClientBrokerSecretName, metav1.GetOptions{})
	if err == nil {
		// Already migrated
		return broker.LocalClientBrokerSecretName, nil
	}

	if err != nil && !errors.IsNotFound(err) {
		return "", status.Error(err, "Error retrieving Broker secret %q", broker.LocalClientBrokerSecretName)
	}

	oldSecret, err := kubeClient.CoreV1().Secrets(ns).Get(ctx, fromSecretName, metav1.GetOptions{})
	if err != nil {
		return "", status.Error(err, "Error retrieving old Broker secret %q", fromSecretName)
	}

	newSecret := *oldSecret
	newSecret.ObjectMeta = metav1.ObjectMeta{
		Name: broker.LocalClientBrokerSecretName,
	}

	_, err = secret.Ensure(ctx, kubeClient, ns, &newSecret)
	if err != nil {
		return "", status.Error(err, "Error creating Broker secret %q", newSecret.Name)
	}

	err = kubeClient.CoreV1().Secrets(ns).Delete(ctx, oldSecret.Name, metav1.DeleteOptions{})
	if err != nil {
		status.Warning("Error deleting the old broker secret %q: %v", oldSecret.Name, err)
	}

	status.Success("Successfully migrated the Broker secret from %q to %q", fromSecretName, broker.LocalClientBrokerSecretName)

	return newSecret.Name, nil
}

func (c *upgradeCommand) upgradeOperator(ctx context.Context, clusterInfo *cluster.Info, repository string, debug bool,
	imageOverride map[string]string, status reporter.Interface,
) error {
	status.Start("Checking if the Operator is installed")
	defer status.End()

	_, err := clusterInfo.ClientProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace).
		Get(ctx, names.OperatorComponent, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return status.Error(err, "Error retrieving Operator deployment")
	}

	status.Start("Upgrading the Operator to %s", c.operatorVersion)

	repositoryInfo := image.NewRepositoryInfo(repository, c.operatorVersion, imageOverride)

	err = operator.Ensure(ctx, status, clusterInfo.ClientProducer, constants.OperatorNamespace, repositoryInfo.GetOperatorImage(), debug,
		&httpProxyConfig)

	return status.Error(err, "Error upgrading the Operator")
}

func (c *upgradeCommand) upgradeConnectivity(ctx context.Context, clusterInfo *cluster.Info, logVersion string,
	status reporter.Interface,
) error {
	if clusterInfo.Submariner != nil {
		status.Start("Upgrading the Connectivity component to %s", logVersion)
		defer status.End()

		clusterInfo.Submariner.Spec.Version = c.submarinerVersion

		var err error

		clusterInfo.Submariner.Spec.BrokerK8sSecret, err = migrateBrokerSecret(ctx, clusterInfo.ClientProducer.ForKubernetes(),
			clusterInfo.Submariner.Spec.BrokerK8sSecret, status)
		if err != nil {
			return err
		}

		err = deploy.SubmarinerFromSpec(ctx, clusterInfo.ClientProducer.ForGeneral(), &clusterInfo.Submariner.Spec)

		return status.Error(err, "Error upgrading the Connectivity component")
	}

	return nil
}

func (c *upgradeCommand) upgradeServiceDiscovery(ctx context.Context, clusterInfo *cluster.Info, logVersion string,
	status reporter.Interface,
) error {
	if clusterInfo.ServiceDiscovery != nil {
		status.Start("Upgrading Service Discovery to %s", logVersion)
		defer status.End()

		clusterInfo.ServiceDiscovery.Spec.Version = c.submarinerVersion

		var err error

		clusterInfo.ServiceDiscovery.Spec.BrokerK8sSecret, err = migrateBrokerSecret(ctx, clusterInfo.ClientProducer.ForKubernetes(),
			clusterInfo.ServiceDiscovery.Spec.BrokerK8sSecret, status)
		if err != nil {
			return err
		}

		err = deploy.ServiceDiscoveryFromSpec(ctx, clusterInfo.ClientProducer.ForGeneral(), &clusterInfo.ServiceDiscovery.Spec)

		return status.Error(err, "Error upgrading Service Discovery")
	}

	return nil
}
