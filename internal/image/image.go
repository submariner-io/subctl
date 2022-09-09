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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	submariner "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/images"
	"github.com/submariner-io/submariner-operator/pkg/names"
	"k8s.io/utils/strings/slices"
)

var validOverrides = []string{
	names.OperatorComponent,
	names.GatewayComponent,
	names.RouteAgentComponent,
	names.GlobalnetComponent,
	names.NetworkPluginSyncerComponent,
	names.ServiceDiscoveryComponent,
	names.LighthouseCoreDNSComponent,
}

func ForOperator(imageVersion, repo string, imageOverrideArr []string) (string, error) {
	if imageVersion == "" {
		imageVersion = submariner.DefaultSubmarinerOperatorVersion
	}

	if repo == "" {
		repo = submariner.DefaultRepo
	}

	imageOverrides, err := GetOverrides(imageOverrideArr)
	if err != nil {
		return "", errors.Wrap(err, "error overriding Operator image")
	}

	return images.GetImagePath(repo, imageVersion, names.OperatorImage, names.OperatorComponent, imageOverrides), nil
}

func GetOverrides(imageOverrideArr []string) (map[string]string, error) {
	imageOverrides := make(map[string]string)

	for _, s := range imageOverrideArr {
		key, value, found := strings.Cut(s, "=")
		if !found {
			return nil, fmt.Errorf("invalid override %s provided. Please use `a=b` syntax", s)
		}

		if !slices.Contains(validOverrides, key) {
			return nil, fmt.Errorf("invalid override component %s provided. Please choose from %q", key, validOverrides)
		}

		imageOverrides[key] = value
	}

	return imageOverrides, nil
}
