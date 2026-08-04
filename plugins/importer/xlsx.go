// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// maxUnzipBytes is the largest total a workbook may decompress to.
const maxUnzipBytes = 64 << 20

// errNotWorkbook reports a zip archive that is not a readable Excel workbook.
var errNotWorkbook = errors.New("the file is a zip archive but not a readable Excel workbook")

// errNoHeader reports an upload whose first worksheet is empty.
var errNoHeader = errors.New("the first worksheet has no header row")

// workbookLimits bound how far an uploaded workbook may decompress.
var workbookLimits = excelize.Options{
	UnzipSizeLimit:    maxUnzipBytes,
	UnzipXMLSizeLimit: maxUnzipBytes,
}

// xlsxParser reads the first worksheet of an Excel workbook.
type xlsxParser struct{}

// parse reads data as the first worksheet of an Excel workbook.
func (xlsxParser) parse(data []byte) (sheet, error) {
	file, err := excelize.OpenReader(bytes.NewReader(data), workbookLimits)
	if err != nil {
		return sheet{}, fmt.Errorf("%w: %w", errNotWorkbook, err)
	}
	defer func() { _ = file.Close() }()
	rows, err := file.Rows(file.GetSheetName(0))
	if err != nil {
		return sheet{}, fmt.Errorf("%w: %w", errNotWorkbook, err)
	}
	defer func() { _ = rows.Close() }()
	return readWorksheet(rows)
}

// readWorksheet collects the column list and data rows a worksheet iterator yields.
func readWorksheet(rows *excelize.Rows) (sheet, error) {
	columns, err := nextRow(rows)
	if err != nil {
		return sheet{}, err
	}
	data, err := readWorksheetRows(rows, len(columns))
	if err != nil {
		return sheet{}, err
	}
	return sheet{columns: columns, rows: data}, nil
}

// nextRow advances the iterator and returns the cells of the row it lands on.
func nextRow(rows *excelize.Rows) ([]string, error) {
	if rows.Next() {
		return rows.Columns()
	}
	if err := rows.Error(); err != nil {
		return nil, err
	}
	return nil, errNoHeader
}

// readWorksheetRows reads the data rows, widening each one to the header width.
func readWorksheetRows(rows *excelize.Rows, width int) ([]row, error) {
	data := make([]row, 0, initialRowCapacity)
	for rows.Next() {
		cells, err := rows.Columns()
		if blank(cells, err) {
			continue
		}
		if len(data) >= maxRows {
			return nil, errTooManyRows
		}
		data = append(data, alignRow(cells, width, err))
	}
	return data, rows.Error()
}

// blank reports whether a worksheet row yielded no cells and no error.
func blank(cells []string, err error) bool {
	return len(cells) == 0 && err == nil
}
