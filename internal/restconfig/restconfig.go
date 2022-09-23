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

	"github.com/coreos/go-semver/semver"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
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
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
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

type loadingRulesAndOverrides struct {
	loadingRules *clientcmd.ClientConfigLoadingRules
	overrides    *clientcmd.ConfigOverrides
}

type Producer struct {
	kubeConfig                    string
	contexts                      []string
	contextPrefixes               []string
	defaultClientConfig           *loadingRulesAndOverrides
	prefixedClientConfigs         map[string]*loadingRulesAndOverrides
	prefixedKubeConfigs           map[string]*string
	inClusterFlag                 bool
	inCluster                     bool
	namespaceFlag                 bool
	contextsFlag                  bool
	deprecatedKubeContextsMessage *string
	defaultNamespace              *string
}

// NewProducer initialises a blank producer which needs to be set up with flags (see SetupFlags).
func NewProducer() *Producer {
	return &Producer{}
}

// NewProducerFrom initialises a producer using the given kubeconfig file and context.
// The context may be empty, in which case the default context will be used.
func NewProducerFrom(kubeConfig, kubeContext string) *Producer {
	producer := &Producer{}

	producer.setupFromConfig(kubeConfig, kubeContext)

	return producer
}

// WithNamespace configures the producer to set up a namespace flag.
// The chosen namespace will be passed to the PerContextFn used to process the context.
func (rcp *Producer) WithNamespace() *Producer {
	rcp.namespaceFlag = true

	return rcp
}

// WithDefaultNamespace configures the producer to set up a namespace flag,
// with the given default value.
// The chosen namespace will be passed to the PerContextFn used to process the context.
func (rcp *Producer) WithDefaultNamespace(defaultNamespace string) *Producer {
	rcp.namespaceFlag = true
	rcp.defaultNamespace = &defaultNamespace

	return rcp
}

// WithPrefixedContext configures the producer to set up flags using the given prefix.
func (rcp *Producer) WithPrefixedContext(prefix string) *Producer {
	rcp.contextPrefixes = append(rcp.contextPrefixes, prefix)

	return rcp
}

// WithContextsFlag configures the producer to allow multiple contexts to be selected with the --contexts flag.
// This is only usable with RunOnAllContexts and will act as a filter on the selected contexts.
func (rcp *Producer) WithContextsFlag() *Producer {
	rcp.contextsFlag = true

	return rcp
}

// WithDeprecatedKubeContexts configures the producer to provide a deprecated --kubecontexts flag as an
// alias for --contexts.
func (rcp *Producer) WithDeprecatedKubeContexts(message string) *Producer {
	rcp.deprecatedKubeContextsMessage = &message

	return rcp
}

// WithInClusterFlag configures the producer to handle an --in-cluster flag, requesting the use
// of a Kubernetes-provided context.
func (rcp *Producer) WithInClusterFlag() *Producer {
	rcp.inClusterFlag = true

	return rcp
}

// SetupFlags configures the given flags to control the producer settings.
func (rcp *Producer) SetupFlags(flags *pflag.FlagSet) {
	if rcp.inClusterFlag {
		flags.BoolVar(&rcp.inCluster, "in-cluster", false, "use the in-cluster configuration to connect to Kubernetes")
	}

	// The base loading rules are shared across all clientcmd setups.
	// This means that alternative kubeconfig setups (remoteconfig etc.) need to
	// be handled manually, but there's no way around that if we want to allow
	// both --kubeconfig and --remoteconfig (only one set of flags can be configured
	// for a given prefix).
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	flags.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "", "absolute path(s) to the kubeconfig file(s)")

	// TODO Alternate kubeconfigs are tracked separately

	// Default prefix
	rcp.defaultClientConfig = rcp.setupContextFlags(loadingRules, flags, "")

	// Support deprecated --kubecontext; TODO remove in 0.15
	legacyFlags := clientcmd.RecommendedConfigOverrideFlags("kube")
	legacyFlags.CurrentContext.BindStringFlag(flags, &rcp.defaultClientConfig.overrides.CurrentContext)
	_ = flags.MarkDeprecated("kubecontext", "use --context instead")

	// Multiple contexts (only on the default prefix)
	if rcp.contextsFlag {
		flags.StringSliceVar(&rcp.contexts, "contexts", nil, "comma-separated list of contexts to use")
	}

	if rcp.deprecatedKubeContextsMessage != nil {
		// Support deprecated --kubecontexts; TODO remove in 0.15
		flags.StringSliceVar(&rcp.contexts, "kubecontexts", nil, "comma-separated list of contexts to use")
		_ = flags.MarkDeprecated("kubecontexts", *rcp.deprecatedKubeContextsMessage)
	}

	// Other prefixes
	rcp.prefixedClientConfigs = make(map[string]*loadingRulesAndOverrides, len(rcp.contextPrefixes))
	rcp.prefixedKubeConfigs = make(map[string]*string, len(rcp.contextPrefixes))

	for _, prefix := range rcp.contextPrefixes {
		rcp.prefixedKubeConfigs[prefix] = flags.String(prefix+"config", "", "absolute path(s) to the "+prefix+" kubeconfig file(s)")
		rcp.prefixedClientConfigs[prefix] = rcp.setupContextFlags(loadingRules, flags, prefix)
	}
}

func (rcp *Producer) setupContextFlags(
	loadingRules *clientcmd.ClientConfigLoadingRules, flags *pflag.FlagSet, prefix string,
) *loadingRulesAndOverrides {
	// Default un-prefixed context
	overrides := clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	kflags := clientcmd.RecommendedConfigOverrideFlags(prefix)

	if !rcp.namespaceFlag {
		// Drop the namespace flag (an empty long name disables a flag)
		kflags.ContextOverrideFlags.Namespace.LongName = ""
	} else if prefix != "" {
		// Avoid attempting to define "-n" twice
		kflags.ContextOverrideFlags.Namespace.ShortName = ""
	}

	clientcmd.BindOverrideFlags(&overrides, flags, kflags)

	return &loadingRulesAndOverrides{
		loadingRules: loadingRules,
		overrides:    &overrides,
	}
}

func (rcp *Producer) setupFromConfig(kubeConfig, kubeContext string) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeConfig

	overrides := clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	rcp.defaultClientConfig = &loadingRulesAndOverrides{
		loadingRules: loadingRules,
		overrides:    &overrides,
	}
}

type AllContextFn func(clusterInfos []*cluster.Info, namespaces []string, status reporter.Interface) error

type PerContextFn func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error

// RunOnSelectedContext runs the given function on the selected context.
func (rcp *Producer) RunOnSelectedContext(function PerContextFn, status reporter.Interface) error {
	if rcp.inCluster {
		return runInCluster(function, status)
	}

	if rcp.defaultClientConfig == nil {
		// If we get here, no context was set up, which means SetupFlags() wasn't called
		return status.Error(errors.New("no context provided (this is a programming error)"), "")
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rcp.defaultClientConfig.loadingRules, rcp.defaultClientConfig.overrides)

	restConfig, err := getRestConfigFromConfig(clientConfig, rcp.defaultClientConfig.overrides)
	if err != nil {
		return status.Error(err, "error retrieving the default configuration")
	}

	clusterInfo, err := cluster.NewInfo(restConfig.ClusterName, restConfig.Config)
	if err != nil {
		return status.Error(err, "error building the cluster.Info for the default configuration")
	}

	namespace, overridden, err := clientConfig.Namespace()
	if err != nil {
		return status.Error(err, "error retrieving the namespace for the default configuration")
	}

	if !overridden && rcp.defaultNamespace != nil {
		namespace = *rcp.defaultNamespace
	}

	return function(clusterInfo, namespace, status)
}

func runInCluster(function PerContextFn, status reporter.Interface) error {
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
	return function(clusterInfo, "", status)
}

// RunOnSelectedContext runs the given function on the selected prefixed context.
// Returns true if there was a selected prefix context, false otherwise.
func (rcp *Producer) RunOnSelectedPrefixedContext(prefix string, function PerContextFn, status reporter.Interface) (bool, error) {
	clientConfig, ok := rcp.prefixedClientConfigs[prefix]
	if ok {
		loadingRules := clientConfig.loadingRules

		// If the user specified a kubeconfig for this prefix, use that instead
		contextKubeConfig, ok := rcp.prefixedKubeConfigs[prefix]
		if ok && contextKubeConfig != nil && *contextKubeConfig != "" {
			loadingRules.ExplicitPath = *contextKubeConfig
		}

		// Has the user actually specified a value for the prefixed context?
		if loadingRules.ExplicitPath == "" && areOverridesEmpty(clientConfig.overrides) {
			return false, nil
		}

		contextClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, clientConfig.overrides)

		restConfig, err := getRestConfigFromConfig(contextClientConfig, clientConfig.overrides)
		if err != nil {
			return true, status.Error(err, "error retrieving the configuration for prefix %s", prefix)
		}

		clusterInfo, err := cluster.NewInfo(restConfig.ClusterName, restConfig.Config)
		if err != nil {
			return true, status.Error(err, "error building the cluster.Info for the configuration for prefix %s", prefix)
		}

		namespace, overridden, err := contextClientConfig.Namespace()
		if err != nil {
			return true, status.Error(err, "error retrieving the namespace for the configuration for prefix %s", prefix)
		}

		if !overridden && rcp.defaultNamespace != nil {
			namespace = *rcp.defaultNamespace
		}

		return true, function(clusterInfo, namespace, status)
	}

	return false, nil
}

// RunOnSelectedContexts runs the given function on all selected contexts, passing them simultaneously.
// This specifically handles the "--contexts" (plural) flag.
// Returns true if there was at least one selected context, false otherwise.
func (rcp *Producer) RunOnSelectedContexts(function AllContextFn, status reporter.Interface) (bool, error) {
	if rcp.inCluster {
		return true, runInCluster(func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
			return function([]*cluster.Info{clusterInfo}, []string{namespace}, status)
		}, status)
	}

	if rcp.defaultClientConfig != nil {
		if len(rcp.contexts) > 0 {
			// Loop over explicitly-chosen contexts
			clusterInfos := []*cluster.Info{}
			namespaces := []string{}

			for _, contextName := range rcp.contexts {
				rcp.defaultClientConfig.overrides.CurrentContext = contextName
				clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
					rcp.defaultClientConfig.loadingRules, rcp.defaultClientConfig.overrides)

				restConfig, err := getRestConfigFromConfig(clientConfig, rcp.defaultClientConfig.overrides)
				if err != nil {
					return true, status.Error(err, "error retrieving the configuration for context %s", contextName)
				}

				clusterInfo, err := cluster.NewInfo(restConfig.ClusterName, restConfig.Config)
				if err != nil {
					return true, status.Error(err, "error building the cluster.Info for context %s", contextName)
				}

				clusterInfos = append(clusterInfos, clusterInfo)

				namespace, overridden, err := clientConfig.Namespace()
				if err != nil {
					return true, status.Error(err, "error retrieving the namespace for context %s", contextName)
				}

				if !overridden && rcp.defaultNamespace != nil {
					namespace = *rcp.defaultNamespace
				}

				namespaces = append(namespaces, namespace)
			}

			return true, function(clusterInfos, namespaces, status)
		}
	}

	return false, nil
}

// RunOnAllContexts runs the given function on all accessible non-prefixed contexts.
// If the user has explicitly selected one or more contexts, only those contexts are used.
// All appropriate contexts are processed, and any errors are aggregated.
// Returns an error if no contexts are found.
func (rcp *Producer) RunOnAllContexts(function PerContextFn, status reporter.Interface) error {
	if rcp.inCluster {
		return runInCluster(function, status)
	}

	if rcp.defaultClientConfig == nil {
		// If we get here, no context was set up, which means SetupFlags() wasn't called
		return status.Error(errors.New("no context provided (this is a programming error)"), "")
	}

	if rcp.defaultClientConfig.overrides.CurrentContext != "" {
		// The user has explicitly chosen a context, use that only
		return rcp.RunOnSelectedContext(function, status)
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rcp.defaultClientConfig.loadingRules, rcp.defaultClientConfig.overrides)

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return status.Error(err, "error retrieving the raw kubeconfig setup")
	}

	contextErrors := []error{}
	processedContexts := 0

	if len(rcp.contexts) > 0 {
		// Loop over explicitly-chosen contexts
		for _, contextName := range rcp.contexts {
			processedContexts++

			chosenContext, ok := rawConfig.Contexts[contextName]
			if !ok {
				contextErrors = append(contextErrors, status.Error(fmt.Errorf("no Kubernetes context found named %s", contextName), ""))

				continue
			}

			contextErrors = append(contextErrors, rcp.overrideContextAndRun(chosenContext.Cluster, contextName, function, status))
		}
	} else {
		// Loop over all accessible contexts
		for contextName, context := range rawConfig.Contexts {
			processedContexts++

			contextErrors = append(contextErrors, rcp.overrideContextAndRun(context.Cluster, contextName, function, status))
		}
	}

	if processedContexts == 0 {
		return status.Error(errors.New("no Kubernetes configuration or context was found"), "")
	}

	return k8serrors.NewAggregate(contextErrors)
}

func (rcp *Producer) overrideContextAndRun(clusterName, contextName string, function PerContextFn, status reporter.Interface) error {
	fmt.Printf("Cluster %q\n", clusterName)

	rcp.defaultClientConfig.overrides.CurrentContext = contextName
	if err := rcp.RunOnSelectedContext(function, status); err != nil {
		return err
	}

	fmt.Println()

	return nil
}

func (rcp *Producer) PopulateTestFramework() {
	framework.TestContext.KubeContexts = rcp.contexts
	if rcp.kubeConfig != "" {
		framework.TestContext.KubeConfig = rcp.kubeConfig
	}
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

func (rcp *Producer) CheckVersionMismatch(cmd *cobra.Command, args []string) error {
	return rcp.RunOnSelectedContext(func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
		crClient := clusterInfo.ClientProducer.ForGeneral()

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
	}, cli.NewReporter())
}

func IfSubmarinerInstalled(functions ...PerContextFn) PerContextFn {
	return func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
		if clusterInfo.Submariner == nil {
			status.Warning(constants.SubmarinerNotInstalled)

			return nil
		}

		aggregateErrors := []error{}

		for _, function := range functions {
			aggregateErrors = append(aggregateErrors, function(clusterInfo, namespace, status))
		}

		return k8serrors.NewAggregate(aggregateErrors)
	}
}

func IfServiceDiscoveryInstalled(functions ...PerContextFn) PerContextFn {
	return func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
		if clusterInfo.ServiceDiscovery == nil {
			status.Warning(constants.ServiceDiscoveryNotInstalled)

			return nil
		}

		aggregateErrors := []error{}

		for _, function := range functions {
			aggregateErrors = append(aggregateErrors, function(clusterInfo, namespace, status))
		}

		return k8serrors.NewAggregate(aggregateErrors)
	}
}

func areOverridesEmpty(overrides *clientcmd.ConfigOverrides) bool {
	return overrides.AuthInfo.ClientCertificate == "" &&
		overrides.AuthInfo.ClientKey == "" &&
		overrides.AuthInfo.Token == "" &&
		overrides.AuthInfo.TokenFile == "" &&
		overrides.AuthInfo.Username == "" &&
		overrides.AuthInfo.Password == "" &&
		overrides.ClusterInfo.CertificateAuthority == "" &&
		overrides.ClusterInfo.ProxyURL == "" &&
		overrides.ClusterInfo.Server == "" &&
		overrides.ClusterInfo.TLSServerName == "" &&
		overrides.CurrentContext == "" &&
		overrides.Context.Cluster == "" &&
		overrides.Context.AuthInfo == "" &&
		overrides.Context.Namespace == ""
}
