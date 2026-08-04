// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"
)

// maxUploadBytes is the largest upload the importer accepts.
const maxUploadBytes = 5 << 20

// maxRows is the largest number of data rows an upload may carry.
const maxRows = 5000

// initialRowCapacity is the row slice capacity every parser starts from.
const initialRowCapacity = 64

// zipMagic opens every zip archive, Excel workbooks included.
var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// errUnsupportedFormat reports an upload that is neither text nor a zip archive.
var errUnsupportedFormat = errors.New("the file is neither a CSV nor an Excel workbook")

// errTooManyRows reports an upload carrying more rows than the importer accepts.
var errTooManyRows = fmt.Errorf("the file carries more than %d rows", maxRows)

// format names the file layout of an upload.
type format string

// The formats the importer parses.
const (
	formatCSV  format = "csv"
	formatXLSX format = "xlsx"
)

// row is one data row of an upload beside the note explaining any repair.
type row struct {
	cells  []string
	reason string
}

// sheet is the column list and data rows a parser recovers from an upload.
type sheet struct {
	columns []string
	rows    []row
}

// parser turns the bytes of one upload into a [sheet].
type parser interface {
	parse(data []byte) (sheet, error)
}

// textual reports whether data is valid UTF-8 free of the NUL bytes marking a binary file.
func textual(data []byte) bool {
	return utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// detect reports the format of data from its leading bytes.
func detect(data []byte) (format, error) {
	if bytes.HasPrefix(data, zipMagic) {
		return formatXLSX, nil
	}
	if textual(data) {
		return formatCSV, nil
	}
	return "", errUnsupportedFormat
}

// parserFor returns the parser reading the given format.
func parserFor(f format) parser {
	if f == formatXLSX {
		return xlsxParser{}
	}
	return csvParser{}
}

// Parse reads an upload into its column list and data rows.
func Parse(data []byte) (sheet, error) {
	f, err := detect(data)
	if err != nil {
		return sheet{}, err
	}
	return parserFor(f).parse(data)
}

// widen returns cells extended with empty strings to width.
func widen(cells []string, width int) []string {
	widened := make([]string, width)
	copy(widened, cells)
	return widened
}

// alignRow widens cells to width and notes any repair the row needed.
func alignRow(cells []string, width int, err error) row {
	switch {
	case err != nil:
		return row{cells: widen(nil, width), reason: fmt.Sprintf("the row is malformed: %v", err)}
	case len(cells) < width:
		return row{cells: widen(cells, width), reason: mismatchReason(len(cells), width)}
	case len(cells) > width:
		return row{cells: cells, reason: mismatchReason(len(cells), width)}
	}
	return row{cells: cells}
}

// mismatchReason describes a row whose cell count differs from the header.
func mismatchReason(got, width int) string {
	return fmt.Sprintf("the row holds %d cells, the header lists %d", got, width)
}
