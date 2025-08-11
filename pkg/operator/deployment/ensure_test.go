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

package deployment_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/operator/deployment"
	"golang.org/x/net/http/httpproxy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testNamespace = "test-namespace"
	testImage     = "submariner-operator:latest"
)

func TestDeployment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Deployment Suite")
}

var _ = Describe("Ensure", func() {
	ctx := context.TODO()

	var kubeClient *k8sfake.Clientset

	BeforeEach(func() {
		kubeClient = k8sfake.NewClientset()

		for _, verb := range []string{"create", "update"} {
			kubeClient.Fake.PrependReactor(verb, "deployments",
				func(a k8stesting.Action) (bool, runtime.Object, error) {
					var d *appsv1.Deployment
					if verb == "create" {
						d = a.(k8stesting.CreateAction).GetObject().(*appsv1.Deployment)
					} else {
						d = a.(k8stesting.UpdateActionImpl).GetObject().(*appsv1.Deployment)
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
	})

	assertDeployment := func() *appsv1.Deployment {
		dep, err := kubeClient.AppsV1().Deployments(constants.OperatorNamespace).Get(ctx, names.OperatorComponent, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		return dep
	}

	When("the deployment doesn't exist", func() {
		It("should create it successfully", func() {
			created, err := deployment.Ensure(ctx, kubeClient, constants.OperatorNamespace, testImage, false, &httpproxy.Config{})
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue())

			dep := assertDeployment()
			Expect(dep.Spec.Template.Spec.ServiceAccountName).To(Equal(names.OperatorComponent))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(testImage))
			Expect(dep.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
			Expect(dep.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-v=1"))

			envVars := dep.Spec.Template.Spec.Containers[0].Env
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name:  "OPERATOR_NAME",
				Value: names.OperatorComponent,
			}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "WATCH_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			}))
		})
	})

	When("the debug flag is specified", func() {
		It("should configure the deployment with debug args", func() {
			_, err := deployment.Ensure(ctx, kubeClient, constants.OperatorNamespace, testImage, true, &httpproxy.Config{})
			Expect(err).NotTo(HaveOccurred())
			Expect(assertDeployment().Spec.Template.Spec.Containers[0].Args).To(ContainElement("-v=3"))
		})
	})

	When("the specified image is local", func() {
		It("should use PullIfNotPresent in the deployment", func() {
			_, err := deployment.Ensure(ctx, kubeClient, constants.OperatorNamespace, "submariner-operator:local", false, &httpproxy.Config{})
			Expect(err).NotTo(HaveOccurred())
			Expect(assertDeployment().Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
		})
	})

	When("HTTP proxy config is specified", func() {
		It("should use PullIfNotPresent in the deployment", func() {
			proxyConfig := &httpproxy.Config{
				HTTPProxy:  "http://proxy:8080",
				HTTPSProxy: "https://proxy:8080",
				NoProxy:    "localhost,127.0.0.1",
			}

			_, err := deployment.Ensure(ctx, kubeClient, constants.OperatorNamespace, testImage, false, proxyConfig)
			Expect(err).NotTo(HaveOccurred())

			envVars := assertDeployment().Spec.Template.Spec.Containers[0].Env
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: proxyConfig.HTTPProxy,
			}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: proxyConfig.HTTPSProxy,
			}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{
				Name:  "NO_PROXY",
				Value: proxyConfig.NoProxy,
			}))
		})
	})

	When("the deployment already exists", func() {
		JustBeforeEach(func() {
			_, err := kubeClient.AppsV1().Deployments(constants.OperatorNamespace).Create(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: constants.OperatorNamespace,
					Name:      names.OperatorComponent,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "submariner-operator:0.20.0",
								},
							},
						},
					},
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update the deployment and return false", func() {
			created, err := deployment.Ensure(ctx, kubeClient, constants.OperatorNamespace, testImage, false, &httpproxy.Config{})
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
			Expect(assertDeployment().Spec.Template.Spec.Containers[0].Image).To(Equal(testImage))
		})
	})

	When("deployment creation fails", func() {
		JustBeforeEach(func() {
			fake.FailOnAction(&kubeClient.Fake, "deployments", "create", nil, false)
		})

		It("should return an error", func() {
			_, err := deployment.Ensure(ctx, kubeClient, constants.OperatorNamespace, testImage, false, &httpproxy.Config{})
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("GetPodLabelSelector", func() {
	var kubeClient *k8sfake.Clientset

	BeforeEach(func() {
		kubeClient = k8sfake.NewClientset()
	})

	When("the deployment exists", func() {
		It("should return the label selector", func() {
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.OperatorComponent,
					Namespace: constants.OperatorNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":  names.OperatorComponent,
								"name": names.OperatorComponent,
							},
						},
					},
				},
			}

			_, err := kubeClient.AppsV1().Deployments(constants.OperatorNamespace).Create(context.TODO(), dep, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			selectorStr, err := deployment.GetPodLabelSelector(kubeClient, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())

			selector, err := labels.Parse(selectorStr)
			Expect(err).NotTo(HaveOccurred())
			Expect(selector.Matches(labels.Set(dep.Spec.Template.Labels))).To(BeTrue())
		})
	})

	When("the deployment doesn't exist", func() {
		It("should return empty string", func() {
			selector, err := deployment.GetPodLabelSelector(kubeClient, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(selector).To(BeEmpty())
		})
	})

	When("retrieving the deployment fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&kubeClient.Fake, "deployments", "get", nil, false)
		})

		It("should return an error", func() {
			_, err := deployment.GetPodLabelSelector(kubeClient, constants.OperatorNamespace)
			Expect(err).To(HaveOccurred())
		})
	})
})
