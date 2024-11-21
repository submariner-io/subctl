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

package gather

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func gatherPodLogs(podLabelSelector string, info *Info, fromContainers ...string) {
	err := func() error {
		pods, err := findPods(info.ClientProducer.ForKubernetes(), podLabelSelector)
		if err != nil {
			return err
		}

		info.Status.Success("Found %d pods matching label selector %q", len(pods.Items), podLabelSelector)

		for i := range pods.Items {
			containers := fromContainers
			if len(containers) == 0 {
				containers = append(getContainerNames(pods.Items[i].Spec.InitContainers),
					getContainerNames(pods.Items[i].Spec.Containers)...)
			}

			for _, container := range containers {
				logName := pods.Items[i].Name
				if len(containers) > 1 {
					logName = logName + "-" + container
				}

				info.Summary.PodLogs = append(info.Summary.PodLogs, outputPodLogs(&pods.Items[i], corev1.PodLogOptions{
					Container: container,
				}, logName, info))
			}
		}

		return nil
	}()
	if err != nil {
		info.Status.Failure("Failed to gather logs for pods matching label selector %q: %s",
			podLabelSelector, err)
	}
}

func getContainerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i := range containers {
		names[i] = containers[i].Name
	}

	return names
}

//nolint:gocritic // hugeParam: podLogOptions - purposely passed by value.
func outputPodLogs(pod *corev1.Pod, podLogOptions corev1.PodLogOptions, logName string, info *Info) (podLogInfo LogInfo) {
	podLogInfo.Namespace = pod.Namespace
	podLogInfo.PodState = pod.Status.Phase
	podLogInfo.PodName = pod.Name
	podLogInfo.NodeName = pod.Spec.NodeName

	err := outputPreviousPodLog(pod, podLogOptions, logName, info, &podLogInfo)
	if err != nil {
		info.Status.Failure("Error outputting previous log for pod %q: %v", pod.Name, err)
	}

	err = outputCurrentPodLog(pod, podLogOptions, logName, info, &podLogInfo)
	if err != nil {
		info.Status.Failure("Error outputting current log for pod %q: %v", pod.Name, err)
	}

	return podLogInfo
}

func writePodLogToFile(logStream io.ReadCloser, info *Info, podName, fileExtension string) (string, error) {
	logs, err := getLogFromStream(logStream)
	if err != nil {
		return "", err
	}

	logs = scrubSensitiveData(info, logs)

	return writeLogToFile(logs, podName, info, fileExtension)
}

func getLogFromStream(logStream io.ReadCloser) (string, error) {
	logs := new(bytes.Buffer)

	_, err := io.Copy(logs, logStream)
	if err != nil {
		return "", errors.WithMessage(err, "error copying the log stream")
	}

	return logs.String(), nil
}

func writeLogToFile(data, podName string, info *Info, fileExtension string) (string, error) {
	fileName := escapeFileName(podName) + fileExtension
	filePath := filepath.Join(info.DirName, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		return "", errors.WithMessagef(err, "error opening file %s", filePath)
	}
	defer f.Close()

	_, err = f.WriteString(data)
	if err != nil {
		return "", errors.WithMessagef(err, "error writing to file %s", filePath)
	}

	return fileName, nil
}

func findPods(clientSet kubernetes.Interface, byLabelSelector string) (*corev1.PodList, error) {
	pods, err := clientSet.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: byLabelSelector})
	if err != nil {
		return nil, errors.WithMessage(err, "error listing pods")
	}

	return pods, nil
}

//nolint:gocritic // hugeParam: podLogOptions - purposely passed by value.
func outputPreviousPodLog(pod *corev1.Pod, podLogOptions corev1.PodLogOptions, logName string, info *Info, podLogInfo *LogInfo) error {
	podLogOptions.Previous = true
	logRequest := info.ClientProducer.ForKubernetes().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOptions)
	logStream, _ := logRequest.Stream(context.TODO())

	// TODO: Check for error other than "no previous pods found"

	// if no previous pods found, logstream == nil, ignore it
	if logStream != nil {
		info.Status.Warning("Found logs for previous instances of pod %q", pod.Name)

		fileName, err := writePodLogToFile(logStream, info, logName, ".log.prev")
		if err != nil {
			return err
		}

		podLogInfo.LogFileName = append(podLogInfo.LogFileName, fileName)

		defer logStream.Close()
	}

	if len(pod.Status.ContainerStatuses) > 0 {
		podLogInfo.RestartCount = pod.Status.ContainerStatuses[0].RestartCount
	}

	return nil
}

//nolint:gocritic // hugeParam: podLogOptions - purposely passed by value.
func outputCurrentPodLog(pod *corev1.Pod, podLogOptions corev1.PodLogOptions, logName string, info *Info, podLogInfo *LogInfo) error {
	// Running with Previous = false on the same pod
	podLogOptions.Previous = false
	logRequest := info.ClientProducer.ForKubernetes().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOptions)

	logStream, err := logRequest.Stream(context.TODO())
	if err != nil {
		return errors.WithMessage(err, "error opening log stream")
	}

	defer logStream.Close()

	fileName, err := writePodLogToFile(logStream, info, logName, ".log")
	podLogInfo.LogFileName = append(podLogInfo.LogFileName, fileName)

	return err
}

func logPodInfo(info *Info, what, podLabelSelector string, process func(info *Info, pod *corev1.Pod)) {
	err := func() error {
		pods, err := findPods(info.ClientProducer.ForKubernetes(), podLabelSelector)
		if err != nil {
			return err
		}

		info.Status.Success("Gathering %s from %d pods matching label selector %q", what, len(pods.Items), podLabelSelector)

		for i := range pods.Items {
			process(info, &pods.Items[i])
		}

		return nil
	}()
	if err != nil {
		info.Status.Failure("Failed to gather %s from pods matching label selector %q: %s",
			what, podLabelSelector, err)
	}
}
