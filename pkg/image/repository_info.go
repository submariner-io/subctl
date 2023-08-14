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

package image

import (
	"github.com/submariner-io/admiral/pkg/names"
	submariner "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/images"
	imagenames "github.com/submariner-io/submariner-operator/pkg/names"
)

type RepositoryInfo struct {
	Name      string
	Version   string
	Overrides map[string]string
}

func NewRepositoryInfo(name, verion string, overrides map[string]string) *RepositoryInfo {
	if name == "" {
		name = submariner.DefaultRepo
	}

	if verion == "" {
		verion = submariner.DefaultSubmarinerOperatorVersion
	}

	repositoryInfo := &RepositoryInfo{
		Name:      name,
		Version:   verion,
		Overrides: overrides,
	}

	return repositoryInfo
}

func (i *RepositoryInfo) GetNettestImage() string {
	return images.GetImagePath(i.Name, i.Version, imagenames.NettestImage, names.NettestComponent, i.Overrides)
}

func (i *RepositoryInfo) GetOperatorImage() string {
	return images.GetImagePath(i.Name, i.Version, imagenames.OperatorImage, names.OperatorComponent, i.Overrides)
}
