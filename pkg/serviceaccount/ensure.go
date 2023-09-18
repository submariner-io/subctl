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
	"github.com/submariner-io/subctl/pkg/secret"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	createdByAnnotation = "kubernetes.io/created-by"
	creatorName         = "subctl"
)

func ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string, sa *corev1.ServiceAccount) (bool, error) {
	result, err := util.CreateOrUpdate(ctx, resource.ForServiceAccount(kubeClient, namespace), sa,
		func(existing runtime.Object) (runtime.Object, error) {
			existing.(*corev1.ServiceAccount).Secrets = nil
			return existing, nil
		})

	return result == util.OperationResultCreated, errors.Wrapf(err, "error creating or updating ServiceAccount %q", sa.Name)
}

//nolint:wrapcheck // No need to wrap errors here.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string, sa *corev1.ServiceAccount,
) (*corev1.ServiceAccount, error) {
	_, err := ensure(ctx, kubeClient, namespace, sa)
	if err != nil {
		return nil, err
	}

	return kubeClient.CoreV1().ServiceAccounts(namespace).Get(ctx, sa.Name, metav1.GetOptions{})
}

// EnsureFromYAML creates the given service account from the YAML representation.
func EnsureFromYAML(ctx context.Context, kubeClient kubernetes.Interface, namespace, yaml string) (bool, error) {
	sa := &corev1.ServiceAccount{}

	err := embeddedyamls.GetObject(yaml, sa)
	if err != nil {
		return false, errors.Wrap(err, "error extracting ServiceAccount resource from YAML")
	}

	return ensure(ctx, kubeClient, namespace, sa)
}

func EnsureTokenSecret(ctx context.Context, client kubernetes.Interface, namespace, saName string) (*corev1.Secret, error) {
	saSecret, err := GetTokenSecretFor(ctx, client, namespace, saName)
	if apierrors.IsNotFound(err) {
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
	}

	if err != nil {
		return nil, errors.Wrapf(err, "error ensuring token secret for service account %q", saName)
	}

	if len(saSecret.Data["token"]) > 0 {
		return saSecret, nil
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

		return len(saSecret.Data["token"]) > 0, nil
	})

	if wait.Interrupted(err) {
		return nil, fmt.Errorf("the token was not generated for secret %q", saSecret.Name)
	}

	return saSecret, err //nolint:wrapcheck // No need to wrap here
}

func GetTokenSecretFor(ctx context.Context, kubeClient kubernetes.Interface, namespace, serviceAccountName string,
) (*corev1.Secret, error) {
	saSecrets, err := kubeClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("type", string(corev1.SecretTypeServiceAccountToken)).String(),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list secrets of type %q in namespace %q",
			corev1.SecretTypeServiceAccountToken, namespace)
	}

	for i := range saSecrets.Items {
		if saSecrets.Items[i].Annotations[corev1.ServiceAccountNameKey] == serviceAccountName {
			return &saSecrets.Items[i], nil
		}
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    corev1.SchemeGroupVersion.Group,
		Resource: "secrets",
	}, serviceAccountName)
}
