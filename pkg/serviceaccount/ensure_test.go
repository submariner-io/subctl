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

package serviceaccount_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

const (
	namespace = "test-namespace"
	saName    = "test-sa"
)

var saYAML = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-sa
`

var _ = Describe("Ensure", func() {
	t := newTestDriver()

	When("the ServiceAccount doesn't exist", func() {
		It("should create it", func(ctx SpecContext) {
			created, err := serviceaccount.Ensure(ctx, t.client, namespace, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: saName,
				},
			})
			Expect(err).To(Succeed())
			actual := t.assertServiceAccount(ctx)
			Expect(created).To(Equal(actual))
		})
	})

	When("an existing ServiceAccount contains a token Secret", func() {
		BeforeEach(func(ctx SpecContext) {
			_, err := t.client.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: saName,
				},
				Secrets: []corev1.ObjectReference{{
					Name: "sa-secret",
				}},
			}, metav1.CreateOptions{})
			Expect(err).To(Succeed())
		})

		It("should remove it", func(ctx SpecContext) {
			updated, err := serviceaccount.Ensure(ctx, t.client, namespace, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: saName,
				},
			})
			Expect(err).To(Succeed())
			Expect(updated.Secrets).To(BeEmpty())
		})
	})
})

var _ = Describe("EnsureFromYAML", func() {
	t := newTestDriver()

	When("the ServiceAccount doesn't exist", func() {
		It("should create it", func(ctx SpecContext) {
			created, err := serviceaccount.EnsureFromYAML(ctx, t.client, namespace, []byte(saYAML))
			Expect(err).To(Succeed())
			Expect(created).To(BeTrue())
			t.assertServiceAccount(ctx)
		})
	})

	When("the ServiceAccount already exists", func() {
		BeforeEach(func(ctx SpecContext) {
			_, err := t.client.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: saName,
				},
			}, metav1.CreateOptions{})
			Expect(err).To(Succeed())
		})

		It("should succeed", func(ctx SpecContext) {
			created, err := serviceaccount.EnsureFromYAML(ctx, t.client, namespace, []byte(saYAML))
			Expect(err).To(Succeed())
			Expect(created).To(BeFalse())
			t.assertServiceAccount(ctx)
		})
	})
})

var _ = Describe("EnsureTokenSecret", func() {
	t := newTestDriver()

	When("the Secret doesn't exist", func() {
		It("should create it", func(ctx SpecContext) {
			secret, err := serviceaccount.EnsureTokenSecret(ctx, t.client, namespace, saName)
			Expect(err).To(Succeed())
			Expect(secret.Type).To(Equal(corev1.SecretTypeServiceAccountToken))
			Expect(secret.Annotations).To(HaveKeyWithValue(corev1.ServiceAccountNameKey, saName))
			t.assertSecret(ctx, secret)
		})
	})

	When("the Secret already exists", func() {
		BeforeEach(func(ctx SpecContext) {
			_, err := t.client.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        saName + "-token-abcde",
					Annotations: map[string]string{corev1.ServiceAccountNameKey: saName},
				},
				Type: corev1.SecretTypeServiceAccountToken,
			}, metav1.CreateOptions{})
			Expect(err).To(Succeed())
		})

		It("should succeed", func(ctx SpecContext) {
			secret, err := serviceaccount.EnsureTokenSecret(ctx, t.client, namespace, saName)
			Expect(err).To(Succeed())
			t.assertSecret(ctx, secret)
		})
	})
})

type testDriver struct {
	client *fakeclientset.Clientset
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.client = fakeclientset.NewClientset()
		stopCh := make(chan struct{})

		_, informer := cache.NewInformerWithOptions(cache.InformerOptions{
			ListerWatcher: cache.ToListWatcherWithWatchListSemantics(&cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					return t.client.CoreV1().Secrets(namespace).List(context.Background(), options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return t.client.CoreV1().Secrets(namespace).Watch(context.Background(), options)
				},
			}, t.client),
			ObjectType: &corev1.Secret{},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj any) {
					s := obj.(*corev1.Secret)
					s.Data = map[string][]byte{"token": {1, 2, 3}}

					_, err := t.client.CoreV1().Secrets(namespace).Update(context.Background(), s, metav1.UpdateOptions{})
					Expect(err).To(Succeed())
				},
			},
		})

		go informer.Run(stopCh)
		Expect(cache.WaitForCacheSync(stopCh, informer.HasSynced)).To(BeTrue())

		DeferCleanup(func() {
			close(stopCh)
		})
	})

	return t
}

func (t *testDriver) assertServiceAccount(ctx context.Context) *corev1.ServiceAccount {
	sa, err := t.client.CoreV1().ServiceAccounts(namespace).Get(ctx, saName, metav1.GetOptions{})
	Expect(err).To(Succeed())

	return sa
}

func (t *testDriver) assertSecret(ctx context.Context, expected *corev1.Secret) {
	list, err := t.client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	Expect(err).To(Succeed())

	var found *corev1.Secret

	for i := range list.Items {
		if list.Items[i].Annotations[corev1.ServiceAccountNameKey] == saName {
			Expect(found).To(BeNil())
			found = &list.Items[i]
		}
	}

	Expect(found).ToNot(BeNil())
	Expect(found).To(Equal(expected))
}
