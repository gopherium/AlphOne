// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/internal/graphres"
)

// legalAnswerRoom is the most the boot answer holds beside the two addresses.
const legalAnswerRoom = 256

// loadLegalLinks reads the addresses of the legal pages, which must fit the answer a caller with no identity gets.
func loadLegalLinks(env gonsole.Env, answerMax int64) (graphres.LegalLinks, error) {
	privacy, err := gonsole.Parse(env, "PRIVACY_URL", "", parseLegalURL)
	if err != nil {
		return graphres.LegalLinks{}, err
	}
	terms, err := gonsole.Parse(env, "TERMS_URL", "", parseLegalURL)
	if err != nil {
		return graphres.LegalLinks{}, err
	}
	held := int64(answerBytes(privacy) + answerBytes(terms) + legalAnswerRoom)
	if held > answerMax {
		return graphres.LegalLinks{}, fmt.Errorf("%s: must be at least %d, the boot read with %s and %s, got %d",
			env.Key("GRAPH_ANONYMOUS_ANSWER_MAX_BYTES"), held, env.Key("PRIVACY_URL"), env.Key("TERMS_URL"), answerMax)
	}
	return graphres.LegalLinks{Privacy: privacy, Terms: terms}, nil
}

// answerBytes returns the bytes an address takes in a graph answer.
func answerBytes(address string) int {
	written, _ := json.Marshal(address)
	return len(written)
}

// parseLegalURL reads the address of a legal page as written, its errors leaving the value out.
func parseLegalURL(value string) (string, error) {
	held, err := url.Parse(value)
	if err != nil {
		return "", errors.New("must be a URL")
	}
	if held.Scheme != "http" && held.Scheme != "https" {
		return "", errors.New("must be an http or https address")
	}
	if held.Hostname() == "" {
		return "", errors.New("must name a host")
	}
	if held.User != nil {
		return "", errors.New("must carry no user or password")
	}
	if !portInRange(held.Port()) {
		return "", errors.New("must carry a port from 1 to 65535")
	}
	return value, nil
}

// portInRange reports whether a port is left out or runs from 1 to 65535.
func portInRange(port string) bool {
	if port == "" {
		return true
	}
	number, err := strconv.ParseUint(port, 10, 16)
	return err == nil && number > 0
}
