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

package fake

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/pkg/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func New() *client.DefaultProducer {
	p := &client.DefaultProducer{
		KubeClient:    k8sfake.NewClientset(),
		DynamicClient: dynamicfake.NewSimpleDynamicClient(scheme.Scheme),
		GeneralClient: controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
			interceptor.Funcs{
				Create: func(ctx context.Context, c ctrlClient.WithWatch, obj ctrlClient.Object, opts ...ctrlClient.CreateOption) error {
					obj.SetUID(uuid.NewUUID())
					return c.Create(ctx, obj, opts...)
				},
			}).Build(),
	}

	AddSecretTokenReactor(&p.KubeClient.(*k8sfake.Clientset).Fake)
	AddDeploymentAvailableReactor(&p.KubeClient.(*k8sfake.Clientset).Fake)

	client.NewProducerFromRestConfig = func(_ *rest.Config) (client.Producer, error) {
		return p, nil
	}

	return p
}

func CreateKubeConfigFile(clientConfig *api.Config) string {
	file, err := os.CreateTemp("", "subctl-unit-test")
	Expect(err).To(Succeed())

	DeferCleanup(func() {
		_ = os.Remove(file.Name())
	})

	Expect(clientcmd.WriteToFile(*clientConfig, file.Name())).To(Succeed())

	return file.Name()
}

func AddDeploymentAvailableReactor(f *testing.Fake) {
	for _, v := range []string{"create", "update"} {
		verb := v
		f.PrependReactor(verb, "deployments",
			func(a testing.Action) (bool, runtime.Object, error) {
				var d *appsv1.Deployment
				if verb == "create" {
					d = a.(testing.CreateAction).GetObject().(*appsv1.Deployment)
				} else {
					d = a.(testing.UpdateActionImpl).GetObject().(*appsv1.Deployment)
				}

				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				}

				return false, nil, nil
			})
	}
}

// AddSecretTokenReactor adds a reactor to automatically generate tokens for secrets.
func AddSecretTokenReactor(f *testing.Fake) {
	f.PrependReactor("create", "secrets",
		func(a testing.Action) (bool, runtime.Object, error) {
			s := a.(testing.CreateAction).GetObject().(*corev1.Secret)
			if s.Data == nil {
				s.Data = map[string][]byte{}
			}

			if len(s.Data["token"]) == 0 {
				if s.Name != "" {
					s.Data["token"] = []byte(s.Name)
				} else {
					s.Data["token"] = []byte(s.GenerateName)
				}
			}

			if len(s.Data["namespace"]) == 0 {
				s.Data["namespace"] = []byte(a.GetNamespace())
			}

			return false, nil, nil
		})
}
