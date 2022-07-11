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

package table

import (
	"fmt"
	"strings"
)

type Column struct {
	Name string

	// Maximum column length (if unspecified then output won't be truncated)
	MaxLength int
}

type Printer struct {
	Columns []Column
	rows    [][]string
}

// Add a new row consisting of values to be printed.
// The value will be cast to string, with special care given to bool and slice.
// It's up to the caller to ensure the number of values match the nubmer of table columns.
func (p *Printer) Add(values ...interface{}) {
	row := make([]string, len(values))

	for i, value := range values {
		if value == nil {
			continue
		}

		switch v := value.(type) {
		case bool:
			row[i] = "no"
			if v {
				row[i] = "yes"
			}
		case []string:
			row[i] = strings.Join(v, ", ")
		default:
			row[i] = fmt.Sprintf("%v", v)
		}
	}

	p.rows = append(p.rows, row)
}

// Empty will be true if there aren't any rows to print.
func (p *Printer) Empty() bool {
	return len(p.rows) == 0
}

// Print out the table; if it's empty then nothing gets printed.
func (p *Printer) Print() {
	if p.Empty() {
		return
	}

	template := p.initTemplate()
	printRow(template, p.columnNames())

	for _, row := range p.rows {
		printRow(template, row)
	}
}

func printRow(template string, row []string) {
	values := make([]interface{}, len(row))
	for i, v := range row {
		values[i] = v
	}

	fmt.Printf(template, values...)
}

func (p *Printer) columnNames() []string {
	columns := make([]string, len(p.Columns))
	for i, column := range p.Columns {
		columns[i] = column.Name
	}

	return columns
}

func (p *Printer) initTemplate() string {
	columnLengths := p.findColumnLengths()

	sprintfTemplate := ""
	for _, length := range columnLengths {
		sprintfTemplate += fmt.Sprintf("%%-%d.%ds", length+3, length)
	}

	return sprintfTemplate + "\n"
}

func (p *Printer) findColumnLengths() []int {
	columnLengths := make([]int, len(p.Columns))
	for i, column := range p.Columns {
		columnLengths[i] = len(column.Name)
	}

	for _, row := range p.rows {
		for i, column := range row {
			maxLength := p.Columns[i].MaxLength
			colLength := len(column)

			// trim the column length if it's going over our maximum (if one is set)
			if maxLength > 0 && colLength > maxLength {
				colLength = maxLength
			}

			if colLength > columnLengths[i] {
				columnLengths[i] = colLength
			}
		}
	}

	return columnLengths
}
