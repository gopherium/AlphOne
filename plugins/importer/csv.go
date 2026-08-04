// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// utf8BOM is the byte order mark Excel writes ahead of a UTF-8 CSV.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// sepHint introduces the delimiter line some exporters write for Excel.
var sepHint = []byte("sep=")

// newline ends the delimiter hint line.
var newline = []byte("\n")

// csvDelimiters are the field delimiters the sniffer chooses between.
var csvDelimiters = []rune{',', ';'}

// csvParser reads delimiter separated text.
type csvParser struct{}

// parse reads data as delimiter separated text.
func (csvParser) parse(data []byte) (sheet, error) {
	body, delimiter := delimiterOf(bytes.TrimPrefix(data, utf8BOM))
	reader := newCSVReader(body, delimiter)
	columns, err := reader.Read()
	if err != nil {
		return sheet{}, fmt.Errorf("the header row is unreadable: %w", err)
	}
	rows, err := readCSVRows(reader, len(columns))
	if err != nil {
		return sheet{}, err
	}
	return sheet{columns: columns, rows: rows}, nil
}

// readCSVRows reads the data rows, recording a malformed record rather than stopping.
func readCSVRows(reader *csv.Reader, width int) ([]row, error) {
	rows := make([]row, 0, initialRowCapacity)
	for len(rows) <= maxRows {
		cells, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		rows = append(rows, alignRow(cells, width, err))
	}
	return nil, errTooManyRows
}

// newCSVReader builds a reader over body that accepts ragged records.
func newCSVReader(body []byte, delimiter rune) *csv.Reader {
	reader := csv.NewReader(bytes.NewReader(body))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	return reader
}

// delimiterOf strips any hint line from body and reports the field delimiter to read it with.
func delimiterOf(body []byte) ([]byte, rune) {
	rest, hinted := cutSepHint(body)
	if headerFields(rest, hinted) > 0 {
		return rest, hinted
	}
	return rest, sniff(rest)
}

// cutSepHint removes a leading delimiter hint line and reports the delimiter it names.
func cutSepHint(body []byte) ([]byte, rune) {
	if !bytes.HasPrefix(body, sepHint) {
		return body, 0
	}
	line, rest, _ := bytes.Cut(body, newline)
	delimiter, _ := utf8.DecodeRune(bytes.TrimSuffix(line[len(sepHint):], []byte("\r")))
	return rest, delimiter
}

// sniff reports the delimiter recovering the most header fields from body.
func sniff(body []byte) rune {
	best, bestFields := csvDelimiters[0], 0
	for _, delimiter := range csvDelimiters {
		if fields := headerFields(body, delimiter); fields > bestFields {
			best, bestFields = delimiter, fields
		}
	}
	return best
}

// headerFields reports how many fields delimiter recovers from the first record of body.
func headerFields(body []byte, delimiter rune) int {
	record, err := newCSVReader(body, delimiter).Read()
	if err != nil {
		return 0
	}
	return len(record)
}
