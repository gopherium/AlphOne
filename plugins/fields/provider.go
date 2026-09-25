// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// booleanTexts maps every text a boolean field accepts to the value it stores.
var booleanTexts = map[string]bool{
	"true": true, "yes": true, "1": true,
	"false": false, "no": false, "0": false,
}

// LiveContactFields lists every field a column may be mapped onto.
func (p *Plugin) LiveContactFields(ctx context.Context) ([]sdk.ContactField, error) {
	definitions, err := p.store.liveDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	listed := make([]sdk.ContactField, 0, len(definitions))
	for _, definition := range definitions {
		listed = append(listed, sdk.ContactField{Name: definition.Name, Label: definition.Label})
	}
	return listed, nil
}

// CheckContactFieldTexts reports whether every text fits the kind the caller's definition declares.
func (p *Plugin) CheckContactFieldTexts(ctx context.Context, values map[string]string) error {
	_, err := p.readTexts(ctx, values)
	return err
}

// WriteContactFieldTexts stores the values the texts describe on one contact.
func (p *Plugin) WriteContactFieldTexts(
	ctx context.Context, contactID uuid.UUID, values map[string]string,
) error {
	read, err := p.readTexts(ctx, values)
	if err != nil {
		return err
	}
	if len(read) == 0 {
		return nil
	}
	return p.store.writeValues(ctx, contactID, read)
}

// readTexts returns the storable values the texts describe under the caller's fields, refusing the rest.
func (p *Plugin) readTexts(ctx context.Context, values map[string]string) (map[string]any, error) {
	written := filledTexts(values)
	if len(written) == 0 {
		return map[string]any{}, nil
	}
	held, err := p.catalog.viewFor(ctx)
	if err != nil {
		return nil, err
	}
	given := make(map[string]any, len(written))
	for name, text := range written {
		given[name] = typedText(held.kinds[name], text)
	}
	checked, err := checkValues(held.kinds, given)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sdk.ErrInvalidFieldText, err)
	}
	return checked, nil
}

// filledTexts returns the texts that carry more than space, trimmed.
func filledTexts(values map[string]string) map[string]string {
	filled := make(map[string]string, len(values))
	for name, text := range values {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			filled[name] = trimmed
		}
	}
	return filled
}

// typedText reads text as the value its kind holds, leaving the rest as written.
func typedText(held kind, text string) any {
	switch held {
	case kindNumber:
		if _, err := strconv.ParseInt(text, 10, 64); err == nil {
			return json.Number(text)
		}
	case kindBoolean:
		if flag, known := booleanTexts[strings.ToLower(text)]; known {
			return flag
		}
	case kindText, kindLongText, kindSelect, kindDate:
	}
	return text
}
