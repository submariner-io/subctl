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

package cloud

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type uintslice []uint16

func (i *uintslice) Type() string {
	return "uintslice"
}

func (i *uintslice) String() string {
	return fmt.Sprintf("%d", *i)
}

func (i *uintslice) Set(value string) error {
	values := strings.Split(value, ",")

	for _, val := range values {
		tmp, err := strconv.ParseUint(val, 0, 16)
		if err != nil {
			return errors.Wrap(err, "Conversion to uint failed")
		}

		if !i.Contains(uint16(tmp)) {
			*i = append(*i, uint16(tmp))
		}
	}

	return nil
}

func (i *uintslice) Contains(value uint16) bool {
	for _, port := range *i {
		if value == port {
			return true
		}
	}

	return false
}

type Ports struct {
	Natt         uint16
	NatDiscovery uint16
	Vxlan        uint16
	Metrics      uintslice
}
