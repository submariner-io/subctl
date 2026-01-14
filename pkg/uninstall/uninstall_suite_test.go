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

package uninstall_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/uninstall"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testClusterName  = "test-cluster"
	testSubmarinerNS = "submariner-ns"
	testBrokerNS     = "broker-ns"
	endpointsCRDName = "endpoints.submariner.io"
	otherCRDName     = "other.example.com"
	testGatewayNode  = "gateway-node"
	otherLabel       = "other-label"
)

var ctx = context.TODO()

var _ = BeforeSuite(func() {
	Expect(operatorv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(apiextensions.AddToScheme(scheme.Scheme)).To(Succeed())

	uninstall.MaxDeletionWait = 500 * time.Millisecond
	uninstall.DeletionCheckInterval = 50 * time.Millisecond
})

func TestUninstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Uninstall Suite")
}

type testDriver struct {
	clients        *client.DefaultProducer
	statusReporter *test.Tracker
	submariner     *operatorv1alpha1.Submariner
	submarinerNS   *corev1.Namespace
	brokerNS       *corev1.Namespace
	broker         *operatorv1alpha1.Broker
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.clients = clientfake.New()
		t.statusReporter = &test.Tracker{Interface: cli.NewReporter()}

		t.submarinerNS = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testSubmarinerNS,
			},
		}

		t.brokerNS = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testBrokerNS,
			},
		}

		t.submariner = &operatorv1alpha1.Submariner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opnames.SubmarinerCrName,
				Namespace: testSubmarinerNS,
			},
		}

		t.broker = &operatorv1alpha1.Broker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-broker",
				Namespace: testBrokerNS,
			},
		}
	})

	JustBeforeEach(func() {
		if t.submarinerNS != nil {
			_, err := t.clients.ForKubernetes().CoreV1().Namespaces().Create(ctx, t.submarinerNS, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		if t.brokerNS != nil {
			_, err := t.clients.ForKubernetes().CoreV1().Namespaces().Create(ctx, t.brokerNS, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		if t.submariner != nil {
			Expect(t.clients.ForGeneral().Create(ctx, t.submariner)).To(Succeed())
		}

		if t.broker != nil {
			Expect(t.clients.ForGeneral().Create(ctx, t.broker)).To(Succeed())
		}

		Expect(t.clients.ForGeneral().Create(ctx, &apiextensions.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpointsCRDName,
			},
		})).To(Succeed())

		Expect(t.clients.ForGeneral().Create(ctx, &apiextensions.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: otherCRDName,
			},
		})).To(Succeed())

		_, err := t.clients.ForKubernetes().RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.OperatorComponent,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.clients.ForKubernetes().RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.OperatorComponent,
			},
			RoleRef: rbacv1.RoleRef{
				Name: names.OperatorComponent,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.clients.ForKubernetes().CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: testGatewayNode,
				Labels: map[string]string{
					constants.SubmarinerGatewayLabel: "true",
					otherLabel:                       "value",
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	return t
}

func (t *testDriver) createOperatorPod(phase corev1.PodPhase) {
	_, err := t.clients.ForKubernetes().CoreV1().Pods(testSubmarinerNS).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "operator-pod",
			Labels: map[string]string{"app": names.OperatorComponent},
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) assertComponentsDeleted() {
	// Verify Submariner resource is deleted
	if t.submariner != nil {
		err := t.clients.ForGeneral().Get(ctx, controllerclient.ObjectKey{
			Namespace: t.submariner.Namespace,
			Name:      t.submariner.Name,
		}, &operatorv1alpha1.Submariner{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}

	// Verify broker namespace is deleted
	if t.broker != nil {
		_, err := t.clients.ForKubernetes().CoreV1().Namespaces().Get(ctx, t.broker.Namespace, metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}

	// Verify Submariner namespace is deleted
	_, err := t.clients.ForKubernetes().CoreV1().Namespaces().Get(ctx, testSubmarinerNS, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())

	// Verify CRDs are deleted
	err = t.clients.ForGeneral().Get(ctx, controllerclient.ObjectKey{Name: endpointsCRDName}, &apiextensions.CustomResourceDefinition{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())

	// Verify cluster roles are deleted
	_, err = t.clients.ForKubernetes().RbacV1().ClusterRoleBindings().Get(ctx, names.OperatorComponent, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())

	_, err = t.clients.ForKubernetes().RbacV1().ClusterRoles().Get(ctx, names.OperatorComponent, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())

	// Verify gateway labels are removed
	updatedNode, err := t.clients.ForKubernetes().CoreV1().Nodes().Get(ctx, testGatewayNode, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(updatedNode.Labels).NotTo(HaveKey(constants.SubmarinerGatewayLabel))
	Expect(updatedNode.Labels).To(HaveKey(otherLabel))

	Expect(t.clients.ForGeneral().Get(ctx, controllerclient.ObjectKey{Name: otherCRDName},
		&apiextensions.CustomResourceDefinition{})).To(Succeed())
}
