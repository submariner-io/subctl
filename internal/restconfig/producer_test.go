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

package restconfig_test

import (
	"context"
	"encoding/base64"
	"os"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/version"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	cluster1Name = "cluster1"
	cluster2Name = "cluster2"
	cluster3Name = "cluster3"
	user1Name    = "user1"
	user2Name    = "user2"
	defaultNS    = "default-ns"
)

var (
	_ = Describe("Producer", func() {
		Context("SetupFlags", testSetupFlags)
		Context("RunOnSelectedContext", testRunOnSelectedContext)
		Context("RunOnSelectedContexts", testRunOnSelectedContexts)
		Context("RunOnAllContexts", testRunOnAllContexts)
		Context("RunOnSelectedPrefixedContext", testRunOnSelectedPrefixedContext)
	})
	_ = Describe("IfConnectivityInstalled", testIfConnectivityInstalled)
	_ = Describe("IfServiceDiscoveryInstalled", testIfServiceDiscoveryInstalled)
	_ = Describe("ForBroker", testForBroker)
)

func testSetupFlags() {
	t := newTestDriver()

	BeforeEach(func() {
		t.setupKubeConfig = false
	})

	It("should add the kubeconfig flag", func() {
		Expect(t.flags.Lookup("kubeconfig")).ToNot(BeNil())
	})

	When("in-cluster is requested", func() {
		BeforeEach(func() {
			t.producer.WithInClusterFlag()
		})

		It("should add the in-cluster flag", func() {
			Expect(t.flags.Lookup(restconfig.InCluster)).ToNot(BeNil())
		})
	})

	When("namespace is requested", func() {
		BeforeEach(func() {
			t.producer.WithNamespace()
		})

		It("should add the namespace flag", func() {
			Expect(t.flags.Lookup(clientcmd.FlagNamespace)).ToNot(BeNil())
		})
	})

	When("namespace is not requested", func() {
		It("should not add the namespace flag", func() {
			Expect(t.flags.Lookup(clientcmd.FlagNamespace)).To(BeNil())
		})
	})

	When("a default namespace is specified", func() {
		BeforeEach(func() {
			t.producer.WithNamespace()
		})

		It("should add the namespace flag", func() {
			Expect(t.flags.Lookup(clientcmd.FlagNamespace)).ToNot(BeNil())
		})
	})

	When("contexts are requested", func() {
		BeforeEach(func() {
			t.producer.WithContextsFlag()
		})

		It("should add the contexts flag", func() {
			Expect(t.flags.Lookup("contexts")).ToNot(BeNil())
		})
	})

	When("a prefix context is requested", func() {
		BeforeEach(func() {
			t.producer.WithPrefixedContext("remote")
		})

		It("should add the prefixed flags", func() {
			Expect(t.flags.Lookup("remoteconfig")).ToNot(BeNil())
			Expect(t.flags.Lookup("remotecontext")).ToNot(BeNil())
		})
	})
}

func testRunOnSelectedContext() {
	t := newTestDriver()

	BeforeEach(func() {
		t.clientConfig.CurrentContext = cluster1Name
		t.expectedProcessed = 1
	})

	It("should run on the current context", func() {
		specifiedStatus := reporter.Stdout()

		err := t.producer.RunOnSelectedContext(func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
			t.actualProcessed++

			Expect(clusterInfo.Name).To(Equal(cluster1Name))
			Expect(namespace).To(Equal(corev1.NamespaceDefault))
			Expect(clusterInfo.Submariner).ToNot(BeNil())
			Expect(status).To(Equal(specifiedStatus))

			return nil
		}, specifiedStatus)

		Expect(err).To(Succeed())
	})

	When("a namespace is configured for the current context", func() {
		BeforeEach(func() {
			t.clientConfig.Contexts[t.clientConfig.CurrentContext].Namespace = "context-ns"
		})

		It("should use the configured namespace", func() {
			err := t.producer.RunOnSelectedContext(func(_ *cluster.Info, namespace string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(namespace).To(Equal(t.clientConfig.Contexts[t.clientConfig.CurrentContext].Namespace))

				return nil
			}, reporter.Stdout())
			Expect(err).To(Succeed())
		})
	})

	When("the namespace flag is specified", func() {
		BeforeEach(func() {
			t.producer.WithNamespace()
		})

		It("should use the specified namespace", func() {
			specifiedNS := "specified-ns"
			Expect(t.flags.Set(clientcmd.FlagNamespace, specifiedNS)).To(Succeed())

			err := t.producer.RunOnSelectedContext(func(_ *cluster.Info, namespace string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(namespace).To(Equal(specifiedNS))

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("a default namespace is specified", func() {
		BeforeEach(func() {
			t.producer.WithDefaultNamespace(defaultNS)
		})

		It("should use the default namespace", func() {
			err := t.producer.RunOnSelectedContext(func(_ *cluster.Info, namespace string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(namespace).To(Equal(defaultNS))

				return nil
			}, reporter.Stdout())
			Expect(err).To(Succeed())
		})
	})

	When("the Submariner version is incompatible with the subctl version", func() {
		BeforeEach(func() {
			t.submariner.Spec.Version = "2.0.0"

			prev := version.Version
			version.Version = "1.0.0"

			DeferCleanup(func() {
				version.Version = prev
			})

			t.expectedProcessed = 0
		})

		It("should return an error", func() {
			err := t.producer.RunOnSelectedContext(t.noopPerContext, reporter.Stdout())
			Expect(err).To(HaveOccurred())
		})
	})

	When("the in-cluster flag is specified", func() {
		BeforeEach(func() {
			t.producer.WithInClusterFlag()
		})

		It("should use the in-cluster config", func() {
			Expect(t.flags.Set(restconfig.InCluster, "true")).To(Succeed())

			err := t.producer.RunOnSelectedContext(func(info *cluster.Info, namespace string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(info.Name).To(Equal(restconfig.InCluster))
				Expect(namespace).To(BeEmpty())

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("there's no current context configured", func() {
		BeforeEach(func() {
			t.clientConfig.CurrentContext = ""
			t.expectedProcessed = 0
		})

		It("should return an error", func() {
			err := t.producer.RunOnSelectedContext(t.noopPerContext, reporter.Stdout())
			Expect(err).To(HaveOccurred())
		})
	})
}

func testRunOnSelectedContexts() {
	t := newTestDriver()

	var (
		specifiedContexts    []string
		configuredNamespaces []string
	)

	BeforeEach(func() {
		t.producer.WithContextsFlag()

		specifiedContexts = []string{cluster2Name, cluster3Name}
		t.expectedProcessed = 1
	})

	JustBeforeEach(func() {
		sort.Strings(specifiedContexts)
		Expect(t.flags.Set("contexts", strings.Join(specifiedContexts, ","))).To(Succeed())
	})

	It("should run on the specified contexts", func() {
		specifiedStatus := reporter.Stdout()

		wasRun, err := t.producer.RunOnSelectedContexts(
			func(clusterInfos []*cluster.Info, namespaces []string, status reporter.Interface) error {
				t.actualProcessed++

				actualContexts := make([]string, len(clusterInfos))
				for i := range clusterInfos {
					actualContexts[i] = clusterInfos[i].Name
				}

				sort.Strings(actualContexts)
				Expect(actualContexts).To(Equal(specifiedContexts))

				for _, ns := range namespaces {
					Expect(ns).To(Equal(corev1.NamespaceDefault))
				}

				Expect(status).To(Equal(specifiedStatus))

				return nil
			}, specifiedStatus)

		Expect(err).To(Succeed())
		Expect(wasRun).To(BeTrue())
	})

	When("namespaces are configured for the specified contexts", func() {
		BeforeEach(func() {
			configuredNamespaces = make([]string, len(specifiedContexts))
			for i := range specifiedContexts {
				configuredNamespaces[i] = specifiedContexts[i] + "-ns"
				t.clientConfig.Contexts[specifiedContexts[i]].Namespace = configuredNamespaces[i]
			}
		})

		It("should use the configured namespaces", func() {
			_, err := t.producer.RunOnSelectedContexts(func(_ []*cluster.Info, namespaces []string, _ reporter.Interface) error {
				t.actualProcessed++

				sort.Strings(namespaces)
				Expect(configuredNamespaces).To(Equal(namespaces))

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("the namespace flag is specified", func() {
		BeforeEach(func() {
			t.producer.WithNamespace()
		})

		It("should use the specified namespace", func() {
			specifiedNS := "specified-ns"
			Expect(t.flags.Set(clientcmd.FlagNamespace, specifiedNS)).To(Succeed())

			_, err := t.producer.RunOnSelectedContexts(func(_ []*cluster.Info, namespaces []string, _ reporter.Interface) error {
				t.actualProcessed++

				for _, ns := range namespaces {
					Expect(ns).To(Equal(specifiedNS))
				}

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("a default namespace is specified", func() {
		BeforeEach(func() {
			t.producer.WithDefaultNamespace(defaultNS)
		})

		It("should use the default namespace", func() {
			_, err := t.producer.RunOnSelectedContexts(func(_ []*cluster.Info, namespaces []string, _ reporter.Interface) error {
				t.actualProcessed++

				for _, ns := range namespaces {
					Expect(ns).To(Equal(defaultNS))
				}

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("the in-cluster flag is specified", func() {
		BeforeEach(func() {
			t.producer.WithInClusterFlag()
		})

		It("should use the in-cluster config", func() {
			Expect(t.flags.Set(restconfig.InCluster, "true")).To(Succeed())

			_, err := t.producer.RunOnSelectedContexts(func(clusterInfos []*cluster.Info, _ []string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(clusterInfos).To(HaveLen(1))
				Expect(clusterInfos[0].Name).To(Equal(restconfig.InCluster))

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("no contexts are specified", func() {
		BeforeEach(func() {
			specifiedContexts = []string{}
			t.expectedProcessed = 0
		})

		It("should return true", func() {
			_, err := t.producer.RunOnSelectedContexts(func(_ []*cluster.Info, _ []string, _ reporter.Interface) error {
				t.actualProcessed++
				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})
}

func testRunOnAllContexts() {
	t := newTestDriver()

	It("should run on the configured contexts", func() {
		t.expectedProcessed = len(t.clientConfig.Contexts)

		specifiedStatus := reporter.Stdout()

		actualContexts := []string{}

		err := t.producer.RunOnAllContexts(func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
			t.actualProcessed++

			Expect(status).To(Equal(specifiedStatus))
			Expect(namespace).To(Equal(corev1.NamespaceDefault))

			actualContexts = append(actualContexts, clusterInfo.Name)

			return nil
		}, specifiedStatus)

		Expect(err).To(Succeed())

		sort.Strings(actualContexts)
		Expect(actualContexts).To(Equal([]string{cluster1Name, cluster2Name, cluster3Name}))
	})

	When("a single context is specified", func() {
		It("should run on the specified context", func() {
			Expect(t.flags.Set(clientcmd.FlagContext, cluster3Name)).To(Succeed())

			t.expectedProcessed = 1

			err := t.producer.RunOnAllContexts(func(clusterInfo *cluster.Info, namespace string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(clusterInfo.Name).To(Equal(cluster3Name))
				Expect(namespace).To(Equal(corev1.NamespaceDefault))

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})

	When("multiple contexts are specified", func() {
		BeforeEach(func() {
			t.producer.WithContextsFlag()
		})

		It("should run on the specified contexts", func() {
			Expect(t.flags.Set("contexts", cluster1Name+","+cluster3Name)).To(Succeed())

			actualContexts := []string{}

			err := t.producer.RunOnAllContexts(func(clusterInfo *cluster.Info, namespace string, _ reporter.Interface) error {
				Expect(namespace).To(Equal(corev1.NamespaceDefault))

				actualContexts = append(actualContexts, clusterInfo.Name)

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())

			sort.Strings(actualContexts)
			Expect(actualContexts).To(Equal([]string{cluster1Name, cluster3Name}))
		})

		Context("and one isn't valid", func() {
			It("should return an error", func() {
				Expect(t.flags.Set("contexts", "non-existent")).To(Succeed())

				err := t.producer.RunOnAllContexts(func(_ *cluster.Info, _ string, _ reporter.Interface) error {
					return nil
				}, reporter.Stdout())

				Expect(err).To(HaveOccurred())
			})
		})
	})

	When("there's multiple contexts for the same cluster", func() {
		BeforeEach(func() {
			t.expectedProcessed = len(t.clientConfig.Contexts)

			t.clientConfig.Contexts["duplicate"] = &api.Context{
				Cluster:  cluster1Name,
				AuthInfo: user2Name,
			}
		})

		It("should de-duplicate via the associated users", func() {
			actualContexts := []string{}

			err := t.producer.RunOnAllContexts(func(clusterInfo *cluster.Info, _ string, _ reporter.Interface) error {
				t.actualProcessed++

				actualContexts = append(actualContexts, clusterInfo.Name)

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
			Expect(actualContexts).To(ContainElement(cluster1Name))
			Expect(actualContexts).ToNot(ContainElement("duplicate"))
		})
	})

	When("there's no contexts configured", func() {
		BeforeEach(func() {
			t.clientConfig.Contexts = map[string]*api.Context{}
		})

		It("should return an error", func() {
			err := t.producer.RunOnAllContexts(func(_ *cluster.Info, _ string, _ reporter.Interface) error {
				return nil
			}, reporter.Stdout())

			Expect(err).To(HaveOccurred())
		})
	})

	When("the in-cluster flag is specified", func() {
		BeforeEach(func() {
			t.producer.WithInClusterFlag()
		})

		It("should use the in-cluster config", func() {
			Expect(t.flags.Set(restconfig.InCluster, "true")).To(Succeed())

			t.expectedProcessed = 1

			err := t.producer.RunOnAllContexts(func(info *cluster.Info, namespace string, _ reporter.Interface) error {
				t.actualProcessed++

				Expect(info.Name).To(Equal(restconfig.InCluster))
				Expect(namespace).To(BeEmpty())

				return nil
			}, reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})
}

func testRunOnSelectedPrefixedContext() {
	const prefix = "to"

	t := newTestDriver()

	BeforeEach(func() {
		t.producer.WithPrefixedContext(prefix)
		t.expectedProcessed = 1
	})

	JustBeforeEach(func() {
		Expect(t.flags.Set(prefix+"context", cluster1Name)).To(Succeed())
	})

	It("should run on the specified prefix context", func() {
		specifiedStatus := reporter.Stdout()

		wasRun, err := t.producer.RunOnSelectedPrefixedContext(prefix,
			func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
				t.actualProcessed++

				Expect(clusterInfo.Name).To(Equal(cluster1Name))
				Expect(namespace).To(Equal(corev1.NamespaceDefault))
				Expect(status).To(Equal(specifiedStatus))

				return nil
			}, specifiedStatus)

		Expect(err).To(Succeed())
		Expect(wasRun).To(BeTrue())
	})

	When("the prefix namespace flag is specified", func() {
		const defaultPrefixNS = "default-prefix-ns"

		BeforeEach(func() {
			t.producer.WithPrefixedNamespace(prefix, defaultPrefixNS)
		})

		It("should use the specified prefix namespace", func() {
			prefixNS := "prefix-ns"
			Expect(t.flags.Set(prefix+clientcmd.FlagNamespace, prefixNS)).To(Succeed())

			wasRun, err := t.producer.RunOnSelectedPrefixedContext(prefix,
				func(clusterInfo *cluster.Info, namespace string, _ reporter.Interface) error {
					t.actualProcessed++

					Expect(clusterInfo.Name).To(Equal(cluster1Name))
					Expect(namespace).To(Equal(prefixNS))

					return nil
				}, reporter.Stdout())

			Expect(err).To(Succeed())
			Expect(wasRun).To(BeTrue())
		})

		Context("and no prefix namespace is specified", func() {
			It("should use the default prefix namespace", func() {
				wasRun, err := t.producer.RunOnSelectedPrefixedContext(prefix,
					func(clusterInfo *cluster.Info, namespace string, _ reporter.Interface) error {
						t.actualProcessed++

						Expect(clusterInfo.Name).To(Equal(cluster1Name))
						Expect(namespace).To(Equal(defaultPrefixNS))

						return nil
					}, reporter.Stdout())

				Expect(err).To(Succeed())
				Expect(wasRun).To(BeTrue())
			})
		})
	})

	When("a default namespace is specified", func() {
		BeforeEach(func() {
			t.producer.WithDefaultNamespace(defaultNS)
		})

		It("should use the default namespace", func() {
			wasRun, err := t.producer.RunOnSelectedPrefixedContext(prefix,
				func(clusterInfo *cluster.Info, namespace string, _ reporter.Interface) error {
					t.actualProcessed++

					Expect(clusterInfo.Name).To(Equal(cluster1Name))
					Expect(namespace).To(Equal(defaultNS))

					return nil
				}, reporter.Stdout())

			Expect(err).To(Succeed())
			Expect(wasRun).To(BeTrue())
		})
	})

	When("the prefix kube config is specified", func() {
		const prefixClusterName = "prefix-cluster"

		var prefixConfigFile string

		BeforeEach(func() {
			clientConfig := api.NewConfig()
			clientConfig.Clusters[prefixClusterName] = &api.Cluster{}
			clientConfig.Contexts[prefixClusterName] = &api.Context{
				Cluster: prefixClusterName,
			}

			prefixConfigFile = createKubeConfigFile(clientConfig)
		})

		JustBeforeEach(func() {
			Expect(t.flags.Set(prefix+"context", prefixClusterName)).To(Succeed())
			Expect(t.flags.Set(prefix+"config", prefixConfigFile)).To(Succeed())
		})

		It("should use the prefixed kube config", func() {
			wasRun, err := t.producer.RunOnSelectedPrefixedContext(prefix,
				func(clusterInfo *cluster.Info, namespace string, _ reporter.Interface) error {
					t.actualProcessed++

					Expect(clusterInfo.Name).To(Equal(prefixClusterName))
					Expect(namespace).To(Equal(corev1.NamespaceDefault))

					return nil
				}, reporter.Stdout())

			Expect(err).To(Succeed())
			Expect(wasRun).To(BeTrue())
		})
	})

	When("no prefix context is specified", func() {
		JustBeforeEach(func() {
			Expect(t.flags.Set(prefix+"context", "")).To(Succeed())
			t.expectedProcessed = 0
		})

		It("should return false", func() {
			wasRun, err := t.producer.RunOnSelectedPrefixedContext(prefix, t.noopPerContext, reporter.Stdout())

			Expect(err).To(Succeed())
			Expect(wasRun).To(BeFalse())
		})
	})
}

func testIfConnectivityInstalled() {
	t := newTestDriver()

	BeforeEach(func() {
		t.clientConfig.CurrentContext = cluster1Name
	})

	When("the Submariner resource is present", func() {
		It("should run the context function", func() {
			t.expectedProcessed = 1
			specifiedStatus := reporter.Stdout()

			err := t.producer.RunOnSelectedContext(restconfig.IfConnectivityInstalled(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					t.actualProcessed++

					Expect(clusterInfo.Name).To(Equal(cluster1Name))
					Expect(namespace).To(Equal(corev1.NamespaceDefault))
					Expect(status).To(Equal(specifiedStatus))

					return nil
				}), specifiedStatus)

			Expect(err).To(Succeed())
		})
	})

	When("the Submariner resource is not present", func() {
		BeforeEach(func() {
			t.submariner = nil
		})

		It("should not run the context function", func() {
			err := t.producer.RunOnSelectedContext(restconfig.IfConnectivityInstalled(
				func(_ *cluster.Info, _ string, _ reporter.Interface) error {
					t.actualProcessed++
					return nil
				}), reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})
}

func testIfServiceDiscoveryInstalled() {
	t := newTestDriver()

	BeforeEach(func() {
		t.clientConfig.CurrentContext = cluster1Name
	})

	When("the ServiceDiscovery resource is present", func() {
		BeforeEach(func() {
			Expect(t.fakeClients.GeneralClient.Create(context.TODO(), &v1alpha1.ServiceDiscovery{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opnames.ServiceDiscoveryCrName,
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())
		})

		It("should run the context function", func() {
			t.expectedProcessed = 1
			specifiedStatus := reporter.Stdout()

			err := t.producer.RunOnSelectedContext(restconfig.IfServiceDiscoveryInstalled(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					t.actualProcessed++

					Expect(clusterInfo.Name).To(Equal(cluster1Name))
					Expect(namespace).To(Equal(corev1.NamespaceDefault))
					Expect(status).To(Equal(specifiedStatus))

					return nil
				}), specifiedStatus)

			Expect(err).To(Succeed())
		})
	})

	When("the ServiceDiscovery resource is not present", func() {
		It("should not run the context function", func() {
			err := t.producer.RunOnSelectedContext(restconfig.IfServiceDiscoveryInstalled(
				func(_ *cluster.Info, _ string, _ reporter.Interface) error {
					t.actualProcessed++
					return nil
				}), reporter.Stdout())

			Expect(err).To(Succeed())
		})
	})
}

func testForBroker() {
	const (
		brokerNS  = "brokerNS"
		apiServer = "api-server"
	)

	var (
		apiServerToken = base64.StdEncoding.EncodeToString([]byte("token"))
		dynClient      dynamic.Interface
	)

	BeforeEach(func() {
		dynClient = dynamicfake.NewSimpleDynamicClient(scheme.Scheme)

		resource.NewDynamicClient = func(_ *rest.Config) (dynamic.Interface, error) {
			return dynClient, nil
		}
	})

	When("a Submariner resource is provided", func() {
		It("should return a valid REST config", func() {
			restConfig, ns, err := restconfig.ForBroker(&v1alpha1.Submariner{
				Spec: v1alpha1.SubmarinerSpec{
					BrokerK8sApiServer:       apiServer,
					BrokerK8sApiServerToken:  apiServerToken,
					BrokerK8sRemoteNamespace: brokerNS,
				},
			}, nil)

			Expect(err).To(Succeed())
			Expect(restConfig).ToNot(BeNil())
			Expect(restConfig.Host).To(ContainSubstring(apiServer))
			Expect(restConfig.BearerToken).To(Equal(apiServerToken))
			Expect(ns).To(Equal(brokerNS))
			Expect(err).To(Succeed())
		})
	})

	When("a ServiceDiscovery resource is provided", func() {
		It("should return a valid REST config", func() {
			restConfig, ns, err := restconfig.ForBroker(nil, &v1alpha1.ServiceDiscovery{
				Spec: v1alpha1.ServiceDiscoverySpec{
					BrokerK8sApiServer:       apiServer,
					BrokerK8sApiServerToken:  apiServerToken,
					BrokerK8sRemoteNamespace: brokerNS,
				},
			})

			Expect(err).To(Succeed())
			Expect(restConfig).ToNot(BeNil())
			Expect(restConfig.Host).To(ContainSubstring(apiServer))
			Expect(restConfig.BearerToken).To(Equal(apiServerToken))
			Expect(ns).To(Equal(brokerNS))
			Expect(err).To(Succeed())
		})
	})

	When("no resource is provided", func() {
		It("should succeed", func() {
			_, _, err := restconfig.ForBroker(nil, nil)
			Expect(err).To(Succeed())
		})
	})
}

type testDriver struct {
	producer          *restconfig.Producer
	clientConfig      *api.Config
	flags             *pflag.FlagSet
	setupKubeConfig   bool
	fakeClients       *client.DefaultProducer
	submariner        *v1alpha1.Submariner
	actualProcessed   int
	expectedProcessed int
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.fakeClients = clientfake.New()
		t.setupKubeConfig = true
		t.expectedProcessed = 0
		t.actualProcessed = 0

		t.submariner = &v1alpha1.Submariner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opnames.SubmarinerCrName,
				Namespace: constants.OperatorNamespace,
			},
		}

		t.clientConfig = api.NewConfig()

		for _, context := range []string{cluster1Name, cluster2Name, cluster3Name} {
			t.clientConfig.Clusters[context] = &api.Cluster{}
			t.clientConfig.Contexts[context] = &api.Context{
				Cluster:  context,
				AuthInfo: user1Name,
			}
		}

		t.clientConfig.AuthInfos[user1Name] = &api.AuthInfo{}
		t.clientConfig.AuthInfos[user2Name] = &api.AuthInfo{}

		t.producer = restconfig.NewProducer()
		t.flags = pflag.NewFlagSet("", pflag.ContinueOnError)
	})

	JustBeforeEach(func() {
		if t.setupKubeConfig {
			os.Setenv("KUBECONFIG", createKubeConfigFile(t.clientConfig))
		}

		if t.submariner != nil {
			Expect(t.fakeClients.GeneralClient.Create(context.TODO(), t.submariner)).To(Succeed())
		}

		t.producer.SetupFlags(t.flags)
	})

	AfterEach(func() {
		Expect(t.actualProcessed).To(Equal(t.expectedProcessed))
	})

	return t
}

func (t *testDriver) noopPerContext(_ *cluster.Info, _ string, _ reporter.Interface) error {
	t.actualProcessed++
	return nil
}

func createKubeConfigFile(clientConfig *api.Config) string {
	file, err := os.CreateTemp("", "subctl-unit-test")
	Expect(err).To(Succeed())

	DeferCleanup(func() {
		_ = os.Remove(file.Name())
	})

	Expect(clientcmd.WriteToFile(*clientConfig, file.Name())).To(Succeed())

	return file.Name()
}
