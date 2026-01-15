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

package cluster_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	localCluster  = "local-cluster"
	remoteCluster = "remote-cluster"
)

var ctx = context.TODO()

var _ = BeforeSuite(func() {
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(apiextensionsv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
})

func TestCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster Suite")
}

type testDriver struct {
	clients     *client.DefaultProducer
	submariner  *v1alpha1.Submariner
	serviceDisc *v1alpha1.ServiceDiscovery
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.clients = clientfake.New()

		t.submariner = &v1alpha1.Submariner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opnames.SubmarinerCrName,
				Namespace: constants.OperatorNamespace,
			},
			Spec: v1alpha1.SubmarinerSpec{
				ClusterID:  localCluster,
				Repository: v1alpha1.DefaultRepo,
				Version:    v1alpha1.DefaultSubmarinerVersion,
			},
		}

		t.serviceDisc = &v1alpha1.ServiceDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opnames.ServiceDiscoveryCrName,
				Namespace: constants.OperatorNamespace,
			},
			Spec: v1alpha1.ServiceDiscoverySpec{
				ClusterID:  "test-cluster",
				Repository: v1alpha1.DefaultRepo,
				Version:    v1alpha1.DefaultLighthouseVersion,
			},
		}
	})

	JustBeforeEach(func() {
		if t.submariner != nil {
			Expect(t.clients.ForGeneral().Create(ctx, t.submariner)).To(Succeed())
		}

		if t.serviceDisc != nil {
			Expect(t.clients.ForGeneral().Create(ctx, t.serviceDisc)).To(Succeed())
		}
	})

	return t
}

func (t *testDriver) newInfo() *cluster.Info {
	info, err := cluster.NewInfo(localCluster, nil)
	Expect(err).NotTo(HaveOccurred())

	return info
}

func (t *testDriver) createNode(name string) {
	_, err := t.newInfo().ClientProducer.ForKubernetes().CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createEndpoint(clusterID string) {
	Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterID,
			Namespace: constants.OperatorNamespace,
		},
		Spec: submarinerv1.EndpointSpec{
			ClusterID: clusterID,
		},
	})).To(Succeed())
}
