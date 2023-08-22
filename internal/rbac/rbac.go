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

package rbac

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

func GetClientTokenSecret(ctx context.Context, kubeClient kubernetes.Interface, namespace, serviceAccountName string,
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
