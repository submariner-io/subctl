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

package deployment

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/pkg/deployment"
	"github.com/submariner-io/submariner-operator/pkg/names"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

// Ensure the operator is deployed, and running.
func Ensure(kubeClient kubernetes.Interface, namespace, image string, debug bool) (bool, error) {
	operatorName := names.OperatorComponent
	replicas := int32(1)
	imagePullPolicy := v1.PullAlways

	// If we are running with a local development image, don't try to pull from registry.
	if strings.HasSuffix(image, ":local") {
		imagePullPolicy = v1.PullIfNotPresent
	}

	command := []string{operatorName}
	if debug {
		command = append(command, "-v=3")
	} else {
		command = append(command, "-v=1")
	}

	opDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      operatorName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"name": operatorName}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"name": operatorName},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: operatorName,
					Containers: []v1.Container{
						{
							Name:            operatorName,
							Image:           image,
							Command:         command,
							ImagePullPolicy: imagePullPolicy,
							SecurityContext: &v1.SecurityContext{
								RunAsNonRoot:             pointer.Bool(true),
								AllowPrivilegeEscalation: pointer.Bool(false),
							},
							Env: []v1.EnvVar{
								{
									Name: "WATCH_NAMESPACE", ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								}, {
									Name: "POD_NAME", ValueFrom: &v1.EnvVarSource{
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								}, {
									Name: "OPERATOR_NAME", Value: operatorName,
								},
							},
						},
					},
				},
			},
		},
	}

	created, err := deployment.Ensure(kubeClient, namespace, opDeployment)
	if err != nil {
		return false, errors.Wrap(err, "error creating/updating Deployment")
	}

	err = deployment.AwaitReady(kubeClient, namespace, opDeployment.Name)

	return created, errors.Wrap(err, "error awaiting Deployment ready")
}

func GetPodLabelSelector(kubeClient kubernetes.Interface, namespace string) (string, error) {
	dep, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), names.OperatorComponent, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", nil
	}

	if err != nil {
		return "", errors.Wrap(err, "error retrieving operator deployment")
	}

	return labels.SelectorFromSet(dep.Spec.Template.ObjectMeta.Labels).String(), nil
}
