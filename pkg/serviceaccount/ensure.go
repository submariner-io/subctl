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

package serviceaccount

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/subctl/internal/rbac"
	"github.com/submariner-io/subctl/pkg/secret"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	createdByAnnotation = "kubernetes.io/created-by"
	creatorName         = "subctl"
)

// ensureFromYAML creates the given service account.
func ensureFromYAML(ctx context.Context, kubeClient kubernetes.Interface, namespace, yaml string) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{}

	err := embeddedyamls.GetObject(yaml, sa)
	if err != nil {
		return nil, err //nolint:wrapcheck // No need to wrap errors here.
	}

	err = ensure(ctx, kubeClient, namespace, sa)
	if err != nil {
		return nil, err
	}

	return sa, err
}

//nolint:wrapcheck // No need to wrap errors here.
func ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string, sa *corev1.ServiceAccount) error {
	_, err := util.CreateOrUpdate(ctx, resource.ForServiceAccount(kubeClient, namespace), sa,
		func(existing runtime.Object) (runtime.Object, error) {
			existing.(*corev1.ServiceAccount).Secrets = nil
			return existing, nil
		})

	return err
}

//nolint:wrapcheck // No need to wrap errors here.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string, sa *corev1.ServiceAccount,
) (*corev1.ServiceAccount, error) {
	err := ensure(ctx, kubeClient, namespace, sa)
	if err != nil {
		return nil, err
	}

	_, err = EnsureSecretFromSA(ctx, kubeClient, sa.Name, namespace)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret for broker SA")
	}

	return kubeClient.CoreV1().ServiceAccounts(namespace).Get(ctx, sa.Name, metav1.GetOptions{})
}

// EnsureFromYAML creates the given service account and secret for it.
func EnsureFromYAML(ctx context.Context, kubeClient kubernetes.Interface, namespace, yaml string) (bool, error) {
	sa, err := ensureFromYAML(ctx, kubeClient, namespace, yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning the ServiceAccount resource")
	}

	saSecret, err := EnsureSecretFromSA(ctx, kubeClient, sa.Name, namespace)
	if err != nil {
		return false, errors.Wrap(err, "error creating secret for ServiceAccount resource")
	}

	return sa != nil && saSecret != nil, nil
}

func EnsureSecretFromSA(ctx context.Context, client kubernetes.Interface, saName, namespace string) (*corev1.Secret, error) {
	saSecret, err := rbac.GetClientTokenSecret(ctx, client, namespace, saName)
	if err == nil {
		return saSecret, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, err //nolint:wrapcheck // No need to wrap
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-token-", saName),
			Namespace:    namespace,
			Annotations: map[string]string{
				corev1.ServiceAccountNameKey: saName,
				createdByAnnotation:          creatorName,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	saSecret, err = secret.Ensure(ctx, client, newSecret.Namespace, newSecret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create secret for service account %q", saName)
	}

	// Ensure the token has been generated for the secret.
	backoff := wait.Backoff{
		Steps:    15,
		Duration: 30 * time.Millisecond,
		Factor:   1.3,
		Jitter:   1,
	}

	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
		saSecret, err = client.CoreV1().Secrets(namespace).Get(ctx, saSecret.Name, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrapf(err, "error getting secret %q", saSecret.Name)
		}

		if len(saSecret.Data["token"]) == 0 {
			return false, nil
		}

		return true, nil
	})

	if wait.Interrupted(err) {
		return nil, fmt.Errorf("the token was not generated for secret %q", saSecret.Name)
	}

	return saSecret, err //nolint:wrapcheck // No need to wrap here
}
