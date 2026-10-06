// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strings"
)

// fakeResolver returns a resolver answering each name in answers with its address and every other name as unknown.
func fakeResolver(answers map[string]netip.Addr) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			go answerLookup(server, answers)
			return client, nil
		},
	}
}

// answerLookup reads one length framed query from conn and writes back its answer.
func answerLookup(conn net.Conn, answers map[string]netip.Addr) {
	defer func() { _ = conn.Close() }()
	var size [2]byte
	if _, err := io.ReadFull(conn, size[:]); err != nil {
		return
	}
	query := make([]byte, binary.BigEndian.Uint16(size[:]))
	if _, err := io.ReadFull(conn, query); err != nil {
		return
	}
	reply := lookupReply(query, answers)
	_, _ = conn.Write(binary.BigEndian.AppendUint16(nil, uint16(len(reply))))
	_, _ = conn.Write(reply)
}

// lookupReply builds the reply to one query, an address record when answers holds the name in the family asked.
func lookupReply(query []byte, answers map[string]netip.Addr) []byte {
	end := 12
	var labels []string
	for query[end] != 0 {
		size := int(query[end])
		labels = append(labels, string(query[end+1:end+1+size]))
		end += size + 1
	}
	end += 5
	kind := binary.BigEndian.Uint16(query[end-4:])
	reply := append([]byte{query[0], query[1], 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}, query[12:end]...)
	address, known := answers[strings.Join(labels, ".")]
	if !known {
		reply[3] = 0x83
		return reply
	}
	if address.Is4() != (kind == 1) {
		return reply
	}
	record := address.AsSlice()
	reply[7] = 1
	reply = append(reply, 0xc0, 0x0c, query[end-4], query[end-3], 0, 1, 0, 0, 0, 60, 0, byte(len(record)))
	return append(reply, record...)
}
