// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"bufio"
	"bytes"
	"cmp"
	"maps"
	"net/textproto"

	"github.com/jhillyerd/enmime/v2"
	"github.com/jhillyerd/enmime/v2/mediatype"
)

// ReadHeaders reads the whole fields within the header cap of the top header block of raw.
func ReadHeaders(raw []byte, limits Limits) (*Message, error) {
	if !limits.set() {
		return nil, ErrLimitsUnset
	}
	block := headerBlock(raw)
	if len(block) > limits.MaxHeaderBytes {
		block = wholeFields(block, limits.MaxHeaderBytes)
	}
	header, _ := enmime.ReadHeader(bufio.NewReader(bytes.NewReader(block)), ignoredProblems{})
	for _, name := range mediaFieldNames {
		if len(header.Get(name)) > limits.MaxFieldBytes {
			header.Del(name)
		}
	}
	m := fromEnvelope(bodylessEnvelope(header), raw)
	m.root = mediaPart(header)
	return m, nil
}

// wholeFields answers the fields of block that end within limit bytes.
func wholeFields(block []byte, limit int) []byte {
	for end := limit; end >= 0; {
		start := bytes.LastIndexByte(block[:end], '\n') + 1
		if opensField(block[start:]) {
			return block[:start]
		}
		end = start - 1
	}
	return nil
}

// opensField reports whether enmime reads the line opening text as a new field.
func opensField(text []byte) bool {
	line, _, _ := bytes.Cut(text, []byte("\n"))
	header, _ := enmime.ReadHeader(bufio.NewReader(bytes.NewReader(line)), ignoredProblems{})
	return len(header) > 0
}

// mediaPart answers a part holding header, with the type and the file name its media fields give.
func mediaPart(header textproto.MIMEHeader) *enmime.Part {
	part := headerPart(header)
	var typeParams map[string]string
	part.ContentType, typeParams, _, _ = mediatype.Parse(header.Get("Content-Type"))
	_, dispositionParams, _, _ := mediatype.Parse(header.Get("Content-Disposition"))
	part.FileName = cmp.Or(dispositionParams["filename"], typeParams["name"], typeParams["file"])
	return part
}

// bodylessEnvelope answers the envelope of a message with the fields of header, its media fields left out.
func bodylessEnvelope(header textproto.MIMEHeader) *enmime.Envelope {
	fields := maps.Clone(header)
	for _, name := range mediaFieldNames {
		fields.Del(name)
	}
	envelope, _ := enmime.EnvelopeFromPart(headerPart(fields))
	return envelope
}

// headerPart answers a part holding header and no content.
func headerPart(header textproto.MIMEHeader) *enmime.Part {
	part := enmime.NewPart("")
	part.Header = header
	return part
}
