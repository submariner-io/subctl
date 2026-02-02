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

package diagnose_test

import (
	"context"
	"encoding/base64"
	"errors"
	"maps"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	reportertest "github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/syncer/test"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/diagnose"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	localCluster  = "local-cluster"
	remoteCluster = "remote-cluster"
)

var errFake = errors.New("fake error")

var _ = BeforeSuite(func() {
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(mcsv1a1.Install(scheme.Scheme)).To(Succeed())

	scheme.Scheme.AddKnownTypeWithName(diagnose.CalicoGVR.GroupVersion().WithKind("IPPoolList"), &unstructured.UnstructuredList{})

	framework.TestContext.OperationTimeout = 1
})

func TestDiagnose(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Diagnose Suite")
}

type testDriver struct {
	fakeProducer     *client.DefaultProducer
	submariner       *v1alpha1.Submariner
	serviceDiscovery *v1alpha1.ServiceDiscovery
	statusTracker    *reportertest.Tracker
}

func (t *testDriver) createResource(obj controllerclient.Object) {
	err := t.fakeProducer.GeneralClient.Create(context.TODO(), obj)
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createServiceExport(se *mcsv1a1.ServiceExport) {
	test.CreateResource(t.fakeProducer.DynamicClient.Resource(
		gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceexports")).Namespace(se.Namespace), se)
}

func (t *testDriver) createServiceImport(si *mcsv1a1.ServiceImport) {
	test.CreateResource(t.fakeProducer.DynamicClient.Resource(
		gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceimports")).Namespace(si.Namespace), si)
}

func (t *testDriver) createService(svc *corev1.Service) {
	_, err := t.fakeProducer.KubeClient.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) creatEndpointSlice(eps *discoveryv1.EndpointSlice) {
	_, err := t.fakeProducer.KubeClient.DiscoveryV1().EndpointSlices(eps.Namespace).Create(context.TODO(), eps, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createNamespace(name string) {
	_, err := t.fakeProducer.KubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	test.CreateResource(t.fakeProducer.DynamicClient.Resource(corev1.SchemeGroupVersion.WithResource("namespaces")),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		})
}

func (t *testDriver) createNode(name string) {
	_, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Create(context.TODO(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createEndpoint(name, clusterID string, subnets []string) {
	t.createResource(&submarinerv1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.submariner.Spec.BrokerK8sRemoteNamespace,
		},
		Spec: submarinerv1.EndpointSpec{
			ClusterID: clusterID,
			Subnets:   subnets,
		},
	})
}

func (t *testDriver) ensureDeployment(name string, desiredReplicas, availableReplicas int32) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &desiredReplicas,
		},
		Status: appsv1.DeploymentStatus{
			AvailableReplicas: availableReplicas,
		},
	}

	_, err := util.CreateOrUpdate(context.TODO(), resource.ForDeployment(t.fakeProducer.KubeClient, constants.OperatorNamespace), deployment,
		func(existing *appsv1.Deployment) (*appsv1.Deployment, error) {
			existing.Spec = deployment.Spec
			existing.Status = deployment.Status

			return existing, nil
		})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) ensureDaemonSet(name string, desired, current int32) {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: desired,
			CurrentNumberScheduled: current,
		},
	}

	_, err := util.CreateOrUpdate(context.TODO(), resource.ForDaemonSet(t.fakeProducer.KubeClient, constants.OperatorNamespace), daemonSet,
		func(existing *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
			existing.Status = daemonSet.Status
			return existing, nil
		})
	Expect(err).NotTo(HaveOccurred())
}

//nolint:gocritic // Ignore huge param
func (t *testDriver) ensurePodWithStatus(name string, labels map[string]string, status corev1.PodStatus) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
			Labels:    map[string]string{"app": name},
		},
		Status: status,
	}

	maps.Copy(pod.Labels, labels)

	_, err := util.CreateOrUpdate(context.TODO(), resource.ForPod(t.fakeProducer.KubeClient, constants.OperatorNamespace), pod,
		func(existing *corev1.Pod) (*corev1.Pod, error) {
			existing.Status = pod.Status
			return existing, nil
		})
	Expect(err).NotTo(HaveOccurred())
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.statusTracker = &reportertest.Tracker{Interface: cli.NewReporter()}
		t.fakeProducer = clientfake.New()
		t.submariner = &v1alpha1.Submariner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      names.SubmarinerCrName,
				Namespace: constants.OperatorNamespace,
			},
			Spec: v1alpha1.SubmarinerSpec{
				ClusterID:                localCluster,
				BrokerK8sApiServer:       "api-server",
				BrokerK8sApiServerToken:  base64.StdEncoding.EncodeToString([]byte("token")),
				BrokerK8sRemoteNamespace: constants.DefaultBrokerNamespace,
				CeIPSecNATTPort:          7319,
			},
			Status: v1alpha1.SubmarinerStatus{
				ClusterID: localCluster,
			},
		}
		t.serviceDiscovery = &v1alpha1.ServiceDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      names.ServiceDiscoveryCrName,
				Namespace: constants.OperatorNamespace,
			},
		}

		resource.NewDynamicClient = func(_ *rest.Config) (dynamic.Interface, error) {
			return t.fakeProducer.DynamicClient, nil
		}

		fake.AddBasicReactors(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake)
	})

	JustBeforeEach(func() {
		t.createNode("master")

		if t.submariner != nil {
			t.createResource(t.submariner)
		}
	})

	return t
}

func (t *testDriver) testFailure(run func(context.Context) error, msgs ...string) {
	It("should fail", func(ctx SpecContext) {
		t.assertFailure(ctx, run, msgs...)
	})
}

func (t *testDriver) assertFailure(ctx context.Context, run func(context.Context) error, msgs ...string) {
	Expect(run(ctx)).NotTo(Succeed())
	t.statusTracker.AssertFailureContainsStrings(msgs...)
}

func (t *testDriver) testSuccess(run func(context.Context) error) {
	It("should succeed", func(ctx SpecContext) {
		t.assertSuccess(ctx, run)
	})
}

func (t *testDriver) assertSuccess(ctx context.Context, run func(context.Context) error) {
	Expect(run(ctx)).To(Succeed())

	t.statusTracker.AssertFailureCount(0)
	t.statusTracker.AssertWarningCount(0)
}

func (t *testDriver) testSuccessWithWarning(run func(context.Context) error, msgs ...string) {
	It("should succeed but emit a warning", func(ctx SpecContext) {
		Expect(run(ctx)).To(Succeed())

		t.statusTracker.AssertFailureCount(0)
		t.statusTracker.AssertWarningCount(1)
		t.statusTracker.AssertWarningContainsStrings(msgs...)
	})
}

func (t *testDriver) testImageRepositoryInfoFailure(before func(), run func(context.Context) error) {
	When("image repository information cannot be determined", func() {
		JustBeforeEach(func() {
			before()
		})

		t.testFailure(run, "repository")
	})
}

func newClusterInfo(ctx context.Context) *cluster.Info {
	info, err := cluster.NewInfo(ctx, localCluster, &rest.Config{})
	Expect(err).NotTo(HaveOccurred())

	return info
}

func newGateway(haStatus submarinerv1.HAStatus, conns ...submarinerv1.Connection) *submarinerv1.Gateway {
	return &submarinerv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: constants.OperatorNamespace,
		},
		Status: submarinerv1.GatewayStatus{
			HAStatus:    haStatus,
			Connections: conns,
		},
	}
}

func newRouteAgent(remoteEPs ...submarinerv1.RemoteEndpoint) *submarinerv1.RouteAgent {
	return &submarinerv1.RouteAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: constants.OperatorNamespace,
		},
		Status: submarinerv1.RouteAgentStatus{
			RemoteEndpoints: remoteEPs,
		},
	}
}
