// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"archive/zip"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/xuri/excelize/v2"
)

// commaCSV is a comma separated export with an aligned header and row.
const commaCSV = "Name,Email\nMaria Perez,maria@example.com\n"

// bomSemicolonCSV is the shape a European locale Excel export carries.
const bomSemicolonCSV = "\ufeffsep=;\r\nName;Phone\r\nMaria Perez;184467235\r\n"

// raggedCSV holds one row short of the header and one row longer than it.
const raggedCSV = "Name,Email,Phone\nMaria Perez\nAna Lopez,ana@example.com,184467235,extra\n"

// duplicateHeaderCSV repeats a column name, which a real export does.
const duplicateHeaderCSV = "Name,Email,Email\nMaria Perez,first@example.com,second@example.com\n"

// quotedSemicolonCSV holds more commas than semicolons inside quoted fields.
const quotedSemicolonCSV = "Name;Note\n\"Perez, Maria\";\"a, b, c\"\n"

// workbookBytes returns an Excel workbook holding the given rows on one sheet.
func workbookBytes(t *testing.T, sheetName string, records [][]any) []byte {
	t.Helper()
	file := excelize.NewFile()
	t.Cleanup(func() { _ = file.Close() })
	if sheetName != "Sheet1" {
		if err := file.SetSheetName("Sheet1", sheetName); err != nil {
			t.Fatalf("renaming sheet: %v", err)
		}
	}
	for i, record := range records {
		cell := "A" + strconv.Itoa(i+1)
		if err := file.SetSheetRow(sheetName, cell, &record); err != nil {
			t.Fatalf("writing row: %v", err)
		}
	}
	var buffer bytes.Buffer
	if err := file.Write(&buffer); err != nil {
		t.Fatalf("writing workbook: %v", err)
	}
	return buffer.Bytes()
}

// zipBytes returns a zip archive holding one named entry.
func zipBytes(t *testing.T, name, content string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("creating zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatalf("writing zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buffer.Bytes()
}

func TestParseReadsACommaSeparatedFile(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte(commaCSV))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	want := sheet{
		columns: []string{"Name", "Email"},
		rows:    []row{{cells: []string{"Maria Perez", "maria@example.com"}}},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(sheet{}, row{})); diff != "" {
		t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseStripsTheByteOrderMarkAndTheSeparatorHint(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte(bomSemicolonCSV))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	want := sheet{
		columns: []string{"Name", "Phone"},
		rows:    []row{{cells: []string{"Maria Perez", "184467235"}}},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(sheet{}, row{})); diff != "" {
		t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseSniffsTheDelimiterPastQuotedCommas(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte(quotedSemicolonCSV))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if diff := cmp.Diff([]string{"Name", "Note"}, got.columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Perez, Maria", "a, b, c"}, got.rows[0].cells); diff != "" {
		t.Errorf("cells mismatch (-want +got):\n%s", diff)
	}
}

func TestParseKeepsEveryColumnWhenAHeaderRepeats(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte(duplicateHeaderCSV))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if diff := cmp.Diff([]string{"Name", "Email", "Email"}, got.columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	want := []string{"Maria Perez", "first@example.com", "second@example.com"}
	if diff := cmp.Diff(want, got.rows[0].cells); diff != "" {
		t.Errorf("cells mismatch (-want +got):\n%s", diff)
	}
}

func TestParseRecordsRaggedRowsRatherThanFailing(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte(raggedCSV))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(got.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(got.rows))
	}
	short, long := got.rows[0], got.rows[1]
	if diff := cmp.Diff([]string{"Maria Perez", "", ""}, short.cells); diff != "" {
		t.Errorf("short row mismatch (-want +got):\n%s", diff)
	}
	if short.reason == "" {
		t.Error("short row reason is empty, want the mismatch recorded")
	}
	if len(long.cells) != 4 {
		t.Errorf("long row kept %d cells, want all 4 preserved", len(long.cells))
	}
	if long.reason == "" {
		t.Error("long row reason is empty, want the mismatch recorded")
	}
}

func TestParseRecordsAMalformedRowRatherThanFailing(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte("Name,Email\nMaria \"Mari\" Perez,maria@example.com\n"))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(got.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(got.rows))
	}
	if diff := cmp.Diff([]string{"", ""}, got.rows[0].cells); diff != "" {
		t.Errorf("cells mismatch (-want +got):\n%s", diff)
	}
	if !strings.Contains(got.rows[0].reason, "malformed") {
		t.Errorf("reason = %q, want the malformed row recorded", got.rows[0].reason)
	}
}

func TestParseSkipsABlankWorksheetRow(t *testing.T) {
	t.Parallel()

	file := excelize.NewFile()
	t.Cleanup(func() { _ = file.Close() })
	header := []any{"Name"}
	if err := file.SetSheetRow("Sheet1", "A1", &header); err != nil {
		t.Fatalf("writing header: %v", err)
	}
	late := []any{"Maria Perez"}
	if err := file.SetSheetRow("Sheet1", "A4", &late); err != nil {
		t.Fatalf("writing row: %v", err)
	}
	var buffer bytes.Buffer
	if err := file.Write(&buffer); err != nil {
		t.Fatalf("writing workbook: %v", err)
	}

	got, err := Parse(buffer.Bytes())

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(got.rows) != 1 {
		t.Errorf("rows = %d, want the blank rows skipped", len(got.rows))
	}
}

func TestParseRejectsAWorkbookWithoutSheets(t *testing.T) {
	t.Parallel()

	data := workbookBytes(t, "Sheet1", [][]any{{"Name"}})
	sheetless := rewriteEntry(t, data, "xl/workbook.xml", func(content string) string {
		start := strings.Index(content, "<sheets>")
		end := strings.Index(content, "</sheets>")
		return content[:start] + content[end+len("</sheets>"):]
	})

	_, err := Parse(sheetless)

	if !errors.Is(err, errNotWorkbook) {
		t.Fatalf("Parse() error = %v, want %v", err, errNotWorkbook)
	}
}

func TestParseReadsASingleColumnFile(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte("Name\nMaria Perez\n"))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if diff := cmp.Diff([]string{"Name"}, got.columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
}

func TestParseAcceptsAHeaderWithoutRows(t *testing.T) {
	t.Parallel()

	got, err := Parse([]byte("Name,Email\n"))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(got.rows) != 0 {
		t.Errorf("rows = %d, want none", len(got.rows))
	}
}

func TestParseRejectsAnEmptyFile(t *testing.T) {
	t.Parallel()

	if _, err := Parse(nil); err == nil {
		t.Fatal("Parse() error = nil, want an unreadable header error")
	}
}

func TestParseRejectsMoreRowsThanTheCap(t *testing.T) {
	t.Parallel()

	var builder strings.Builder
	builder.WriteString("Name\n")
	for range maxRows + 1 {
		builder.WriteString("Maria Perez\n")
	}

	_, err := Parse([]byte(builder.String()))

	if !errors.Is(err, errTooManyRows) {
		t.Fatalf("Parse() error = %v, want %v", err, errTooManyRows)
	}
}

func TestParseAcceptsExactlyTheRowCap(t *testing.T) {
	t.Parallel()

	var builder strings.Builder
	builder.WriteString("Name\n")
	for range maxRows {
		builder.WriteString("Maria Perez\n")
	}

	got, err := Parse([]byte(builder.String()))

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(got.rows) != maxRows {
		t.Errorf("rows = %d, want %d", len(got.rows), maxRows)
	}
}

func TestParseReadsTheFirstWorksheetOfAWorkbook(t *testing.T) {
	t.Parallel()

	data := workbookBytes(t, "Contacts", [][]any{
		{"Name", "Phone"},
		{"Maria Perez", 184467235},
	})

	got, err := Parse(data)

	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if diff := cmp.Diff([]string{"Name", "Phone"}, got.columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Maria Perez", "184467235"}, got.rows[0].cells); diff != "" {
		t.Errorf("cells mismatch (-want +got):\n%s", diff)
	}
}

func TestParseRejectsAWorkbookWithoutRows(t *testing.T) {
	t.Parallel()

	data := workbookBytes(t, "Contacts", nil)

	_, err := Parse(data)

	if !errors.Is(err, errNoHeader) {
		t.Fatalf("Parse() error = %v, want %v", err, errNoHeader)
	}
}

func TestParseRejectsMoreWorksheetRowsThanTheCap(t *testing.T) {
	t.Parallel()

	records := make([][]any, 0, maxRows+2)
	records = append(records, []any{"Name"})
	for range maxRows + 1 {
		records = append(records, []any{"Maria Perez"})
	}

	_, err := Parse(workbookBytes(t, "Sheet1", records))

	if !errors.Is(err, errTooManyRows) {
		t.Fatalf("Parse() error = %v, want %v", err, errTooManyRows)
	}
}

func TestParseRejectsAZipThatIsNotAWorkbook(t *testing.T) {
	t.Parallel()

	_, err := Parse(zipBytes(t, "notes.txt", "Maria Perez"))

	if !errors.Is(err, errNotWorkbook) {
		t.Fatalf("Parse() error = %v, want %v", err, errNotWorkbook)
	}
}

func TestParseRejectsBinaryContent(t *testing.T) {
	t.Parallel()

	tests := map[string][]byte{
		"png header":   {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
		"latin1 bytes": {0x4E, 0x61, 0x6D, 0x65, 0xF1, 0x0A},
		"embedded nul": {0x4E, 0x61, 0x6D, 0x65, 0x00, 0x0A},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := Parse(data); !errors.Is(err, errUnsupportedFormat) {
				t.Fatalf("Parse() error = %v, want %v", err, errUnsupportedFormat)
			}
		})
	}
}

func TestParseRejectsAWorksheetRowBeyondTheSheetLimit(t *testing.T) {
	t.Parallel()

	data := workbookBytes(t, "Sheet1", [][]any{{"Name"}})
	hostile := rewriteSheetXML(t, data, `<row r="1"`, `<row r="1048577"`)

	_, err := Parse(hostile)

	if err == nil {
		t.Fatal("Parse() error = nil, want the row limit reported")
	}
}

// rewriteSheetXML returns the workbook with one substitution applied to its worksheet part.
func rewriteSheetXML(t *testing.T, data []byte, old, replacement string) []byte {
	t.Helper()
	return rewriteEntry(t, data, "xl/worksheets/sheet1.xml", func(content string) string {
		return strings.Replace(content, old, replacement, 1)
	})
}

// rewriteEntry returns the archive with one named entry rewritten.
func rewriteEntry(t *testing.T, data []byte, name string, rewrite func(string) string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("reading workbook: %v", err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range reader.File {
		content := readZipEntry(t, file)
		if file.Name == name {
			content = rewrite(content)
		}
		entry, err := writer.Create(file.Name)
		if err != nil {
			t.Fatalf("creating entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("writing entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing workbook: %v", err)
	}
	return buffer.Bytes()
}

// readZipEntry returns the content of one archive entry.
func readZipEntry(t *testing.T, file *zip.File) string {
	t.Helper()
	handle, err := file.Open()
	if err != nil {
		t.Fatalf("opening entry: %v", err)
	}
	defer func() { _ = handle.Close() }()
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(handle); err != nil {
		t.Fatalf("reading entry: %v", err)
	}
	return buffer.String()
}
