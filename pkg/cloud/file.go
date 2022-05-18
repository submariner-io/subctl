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
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func ReadMetadataFile(fileName string, metadata interface{}) error {
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return errors.Wrapf(err, "failed to stat file %q", fileName)
	}

	if fileInfo.IsDir() {
		fileName = filepath.Join(fileName, "metadata.json")
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		return errors.Wrapf(err, "error reading file %q", fileName)
	}

	err = json.Unmarshal(data, metadata)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling data")
	}

	return nil
}
