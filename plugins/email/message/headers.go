// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"bufio"
	"bytes"
	"maps"
	"net/textproto"

	"github.com/jhillyerd/enmime/v2"
	"github.com/jhillyerd/enmime/v2/mediatype"
)

// ReadHeaders reads the top header block of raw up to the header cap, for a message ParseBytes cannot read whole.
func ReadHeaders(raw []byte, limits Limits) (*Message, error) {
	if !limits.set() {
		return nil, ErrLimitsUnset
	}
	block := bytes.NewReader(raw[:min(len(raw), limits.MaxHeaderBytes)])
	header, _ := enmime.ReadHeader(bufio.NewReader(block), ignoredProblems{})
	for _, name := range mediaFieldNames {
		if len(header.Get(name)) > limits.MaxFieldBytes {
			header.Del(name)
		}
	}
	m := fromEnvelope(bodylessEnvelope(header), raw)
	m.root = headerPart(header)
	m.root.ContentType, _, _, _ = mediatype.Parse(header.Get("Content-Type"))
	return m, nil
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
