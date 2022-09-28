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

package restconfig

import (
	"context"
	"fmt"
	"os"

	"github.com/coreos/go-semver/semver"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/stringset"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/version"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	subv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type RestConfig struct {
	Config      *rest.Config
	ClusterName string
}

type configAndOverrides struct {
	config    clientcmd.ClientConfig
	overrides *clientcmd.ConfigOverrides
}

type Producer struct {
	kubeConfig          string
	kubeContext         string
	kubeContexts        []string
	defaultClientConfig *configAndOverrides
	inCluster           bool
	namespaceFlag       bool
}

// NewProducer initialises a blank producer which needs to be set up with flags (see SetupFlags).
func NewProducer() *Producer {
	return &Producer{}
}

// NewProducerFrom initialises a producer using the given kubeconfig file and context.
// The context may be empty, in which case the default context will be used.
func NewProducerFrom(kubeConfig, kubeContext string) *Producer {
	return &Producer{
		kubeConfig:  kubeConfig,
		kubeContext: kubeContext,
	}
}

// WithNamespace configures the producer to set up a namespace flag.
// The chosen namespace will be passed to the PerContextFn used to process the context.
func (rcp *Producer) WithNamespace() *Producer {
	rcp.namespaceFlag = true

	return rcp
}

// SetupFlags configures the given flags to control the producer settings.
func (rcp *Producer) SetupFlags(flags *pflag.FlagSet) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	flags.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "", "absolute path(s) to the kubeconfig file(s)")

	// Default un-prefixed context
	overrides := clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	kflags := clientcmd.RecommendedConfigOverrideFlags("")
	clientcmd.BindOverrideFlags(&overrides, flags, kflags)

	// Support deprecated --kubecontext; TODO remove in 0.15
	legacyFlags := clientcmd.RecommendedConfigOverrideFlags("kube")
	legacyFlags.CurrentContext.BindStringFlag(flags, &overrides.CurrentContext)
	_ = flags.MarkDeprecated("kubecontext", "use --context instead")

	if !rcp.namespaceFlag {
		_ = flags.MarkHidden("namespace")
	}

	rcp.defaultClientConfig = &configAndOverrides{
		config:    clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &overrides),
		overrides: &overrides,
	}
}

type PerContextFn func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error

// RunOnSelectedContext runs the given function on the selected context.
func (rcp *Producer) RunOnSelectedContext(function PerContextFn, status reporter.Interface) error {
	if rcp.inCluster {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return status.Error(err, "error retrieving the in-cluster configuration")
		}

		// In-cluster configurations don't give a cluster name, use "in-cluster"
		clusterInfo, err := cluster.NewInfo("in-cluster", restConfig)
		if err != nil {
			return status.Error(err, "error building the cluster.Info for the in-cluster configuration")
		}

		// In-cluster configurations don't specify a namespace, use the default
		// When using the in-cluster configuration, that's the only configuration we want
		return function(clusterInfo, "default", status)
	}

	if rcp.defaultClientConfig == nil {
		// If we get here, no context was set up, which means SetupFlags() wasn't called
		return status.Error(errors.New("no context provided (this is a programming error)"), "")
	}

	restConfig, err := getRestConfigFromConfig(rcp.defaultClientConfig.config, rcp.defaultClientConfig.overrides)
	if err != nil {
		return status.Error(err, "error retrieving the default configuration")
	}

	clusterInfo, err := cluster.NewInfo(restConfig.ClusterName, restConfig.Config)
	if err != nil {
		return status.Error(err, "error building the cluster.Info for the default configuration")
	}

	namespace, _, err := rcp.defaultClientConfig.config.Namespace()
	if err != nil {
		return status.Error(err, "error retrieving the namespace for the default configuration")
	}

	return function(clusterInfo, namespace, status)
}

func (rcp *Producer) AddKubeConfigFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&rcp.kubeConfig, "kubeconfig", "", "absolute path(s) to the kubeconfig file(s)")
}

// AddKubeContextFlag adds a "kubeconfig" flag and a single "kubecontext" flag that can be used once and only once.
func (rcp *Producer) AddKubeContextFlag(cmd *cobra.Command) {
	rcp.AddKubeConfigFlag(cmd)
	cmd.PersistentFlags().StringVar(&rcp.kubeContext, "kubecontext", "", "kubeconfig context to use")
}

// AddKubeContextMultiFlag adds a "kubeconfig" flag and a "kubecontext" flag that can be specified multiple times (or comma separated).
func (rcp *Producer) AddKubeContextMultiFlag(cmd *cobra.Command, usage string) {
	rcp.AddKubeConfigFlag(cmd)

	if usage == "" {
		usage = "comma-separated list of kubeconfig contexts to use, can be specified multiple times.\n" +
			"If none specified, all contexts referenced by the kubeconfig are used"
	}

	cmd.PersistentFlags().StringSliceVar(&rcp.kubeContexts, "kubecontexts", nil, usage)
}

// AddInClusterConfigFlag adds a flag enabling in-cluster configurations for processes running in pods.
func (rcp *Producer) AddInClusterConfigFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&rcp.inCluster, "in-cluster", false, "use the in-cluster configuration to connect to Kubernetes")
}

func (rcp *Producer) PopulateTestFramework() {
	framework.TestContext.KubeContexts = rcp.kubeContexts
	if rcp.kubeConfig != "" {
		framework.TestContext.KubeConfig = rcp.kubeConfig
	}
}

func (rcp *Producer) MustGetForClusters() []RestConfig {
	configs, err := rcp.getRestConfigs()
	exit.OnErrorWithMessage(err, "Error getting REST Config for cluster")

	return configs
}

func (rcp *Producer) CountRequestedClusters() int {
	if len(rcp.kubeContexts) > 0 {
		// Count unique contexts
		contexts := stringset.New()
		for i := range rcp.kubeContexts {
			contexts.Add(rcp.kubeContexts[i])
		}

		return contexts.Size()
	}
	// Current context or rcp.kubeContext
	return 1
}

func (rcp *Producer) ForCluster() (RestConfig, error) {
	var restConfig RestConfig

	restConfigs, err := rcp.getRestConfigs()
	if err != nil {
		return restConfig, err
	}

	if len(restConfigs) > 0 {
		return restConfigs[0], nil
	}

	return restConfig, errors.New("error getting restconfig")
}

func (rcp *Producer) getRestConfigs() ([]RestConfig, error) {
	if rcp.inCluster {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return []RestConfig{}, errors.Wrap(err, "error retrieving the in-cluster configuration")
		}

		return []RestConfig{{
			Config:      restConfig,
			ClusterName: "in-cluster",
		}}, nil
	}

	var restConfigs []RestConfig

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	rules.ExplicitPath = rcp.kubeConfig

	contexts := []string{}
	if len(rcp.kubeContexts) > 0 {
		contexts = append(contexts, rcp.kubeContexts...)
	} else if len(rcp.kubeContext) > 0 {
		contexts = append(contexts, rcp.kubeContext)
	} else {
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
		rawConfig, err := kubeConfig.RawConfig()
		if err != nil {
			return restConfigs, errors.Wrap(err, "error creating kube config")
		}

		for context := range rawConfig.Contexts {
			contexts = append(contexts, context)
		}
	}

	for _, context := range contexts {
		if context != "" {
			overrides.CurrentContext = context

			config, err := clientConfigAndClusterName(rules, overrides)
			if err != nil {
				return nil, err
			}

			restConfigs = append(restConfigs, config)
		}
	}

	return restConfigs, nil
}

func ForBroker(submariner *v1alpha1.Submariner, serviceDisc *v1alpha1.ServiceDiscovery) (*rest.Config, string, error) {
	var restConfig *rest.Config
	var namespace string
	var err error

	// This is used in subctl; the broker secret isn't available mounted, so we use the old strings for now
	if submariner != nil {
		// Try to authorize against the submariner Cluster resource as we know the CRD should exist and the credentials
		// should allow read access.
		restConfig, _, err = resource.GetAuthorizedRestConfigFromData(submariner.Spec.BrokerK8sApiServer,
			submariner.Spec.BrokerK8sApiServerToken,
			submariner.Spec.BrokerK8sCA,
			&rest.TLSClientConfig{},
			subv1.SchemeGroupVersion.WithResource("clusters"),
			submariner.Spec.BrokerK8sRemoteNamespace)
		namespace = submariner.Spec.BrokerK8sRemoteNamespace
	} else if serviceDisc != nil {
		// Try to authorize against the ServiceImport resource as we know the CRD should exist and the credentials
		// should allow read access.
		restConfig, _, err = resource.GetAuthorizedRestConfigFromData(serviceDisc.Spec.BrokerK8sApiServer,
			serviceDisc.Spec.BrokerK8sApiServerToken,
			serviceDisc.Spec.BrokerK8sCA,
			&rest.TLSClientConfig{},
			gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceimports"),
			serviceDisc.Spec.BrokerK8sRemoteNamespace)
		namespace = serviceDisc.Spec.BrokerK8sRemoteNamespace
	}

	return restConfig, namespace, errors.Wrap(err, "error getting auth rest config")
}

func clientConfigAndClusterName(rules *clientcmd.ClientConfigLoadingRules, overrides *clientcmd.ConfigOverrides) (RestConfig, error) {
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

	return getRestConfigFromConfig(config, overrides)
}

func getRestConfigFromConfig(config clientcmd.ClientConfig, overrides *clientcmd.ConfigOverrides) (RestConfig, error) {
	clientConfig, err := config.ClientConfig()
	if err != nil {
		return RestConfig{}, errors.Wrap(err, "error creating client config")
	}

	raw, err := config.RawConfig()
	if err != nil {
		return RestConfig{}, errors.Wrap(err, "error creating rest config")
	}

	clusterName := clusterNameFromContext(&raw, overrides.CurrentContext)

	if clusterName == nil {
		return RestConfig{}, fmt.Errorf("could not obtain the cluster name from kube config: %#v", raw)
	}

	return RestConfig{Config: clientConfig, ClusterName: *clusterName}, nil
}

func (rcp *Producer) clusterNameFromContext() (*string, error) {
	rawConfig, err := rcp.ClientConfig().RawConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving raw client configuration")
	}

	return clusterNameFromContext(&rawConfig, rcp.kubeContext), nil
}

func clusterNameFromContext(rawConfig *api.Config, overridesContext string) *string {
	if overridesContext == "" {
		// No context provided, use the current context.
		overridesContext = rawConfig.CurrentContext
	}

	configContext, ok := rawConfig.Contexts[overridesContext]
	if !ok {
		return nil
	}

	return &configContext.Cluster
}

func (rcp *Producer) GetClusterID() (string, error) {
	clusterName, err := rcp.clusterNameFromContext()
	if err != nil {
		return "", err
	}

	if clusterName != nil {
		return *clusterName, nil
	}

	return "", nil
}

// ClientConfig returns a clientcmd.ClientConfig to use when communicating with K8s.
func (rcp *Producer) ClientConfig() clientcmd.ClientConfig {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = rcp.kubeConfig

	rules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}

	if rcp.kubeContext != "" {
		overrides.CurrentContext = rcp.kubeContext
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
}

func (rcp *Producer) CheckVersionMismatch(cmd *cobra.Command, args []string) error {
	if rcp.defaultClientConfig != nil {
		// We're using clientcmd kubeconfig flags
		return rcp.RunOnSelectedContext(func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
			return checkVersionMismatch(clusterInfo.ClientProducer.ForGeneral())
		}, cli.NewReporter())
	}

	// Legacy flag handling
	config, err := rcp.ForCluster()
	exit.OnErrorWithMessage(err, "The provided kubeconfig is invalid")

	crClient, err := controllerClient.New(config.Config, controllerClient.Options{})
	exit.OnErrorWithMessage(err, "Error creating client")

	return checkVersionMismatch(crClient)
}

func checkVersionMismatch(crClient controllerClient.Client) error {
	submariner := &v1alpha1.Submariner{}
	err := crClient.Get(context.TODO(), controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      names.SubmarinerCrName,
	}, submariner)

	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		return nil
	}

	exit.OnErrorWithMessage(err, fmt.Sprintf("Error retrieving Submariner object %s", names.SubmarinerCrName))

	if submariner != nil && submariner.Spec.Version != "" {
		subctlVer, _ := semver.NewVersion(version.Version)
		submarinerVer, _ := semver.NewVersion(submariner.Spec.Version)

		if subctlVer != nil && submarinerVer != nil && subctlVer.LessThan(*submarinerVer) {
			return fmt.Errorf(
				"the subctl version %q is older than the deployed Submariner version %q. Please upgrade your subctl version",
				version.Version, submariner.Spec.Version)
		}
	}

	return nil
}

func ConfigureTestFramework(args []string) error {
	// Legacy handling: if arguments are files, assume they are kubeconfigs;
	// otherwise, use contexts from --kubecontexts
	_, err1 := os.Stat(args[0])
	var err2 error

	if len(args) > 1 {
		_, err2 = os.Stat(args[1])
	}

	if err1 != nil || err2 != nil {
		// Something happened (possibly IsNotExist, but we don’t care about specifics)
		return fmt.Errorf("the provided arguments (%v) aren't accessible files", args)
	}

	// The files exist and can be examined without error
	framework.TestContext.KubeConfig = ""
	framework.TestContext.KubeConfigs = args

	// Read the cluster names from the given kubeconfigs
	for _, config := range args {
		rcp := NewProducerFrom(config, "")

		clusterName, err := rcp.clusterNameFromContext()
		if err != nil {
			// nolint:nilerr // This is intentional.
			return nil
		}

		framework.TestContext.ClusterIDs = append(framework.TestContext.ClusterIDs, *clusterName)
	}

	return nil
}
