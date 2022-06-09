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

package subctl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type uint16Slice struct {
	value *[]uint16
}

func (s *uint16Slice) Type() string {
	return "uint16Slice"
}

func (s *uint16Slice) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *uint16Slice) Set(value string) error {
	values := strings.Split(value, ",")

	*s.value = make([]uint16, len(values))

	for i, d := range values {
		u, err := strconv.ParseUint(d, 10, 16)
		if err != nil {
			return errors.Wrap(err, "conversion to uint16 failed")
		}

		(*s.value)[i] = uint16(u)
	}

	return nil
}
