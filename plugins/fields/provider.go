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

// CheckContactFieldTexts reports whether every text fits the kind its definition declares.
func (p *Plugin) CheckContactFieldTexts(_ context.Context, values map[string]string) error {
	_, err := p.readTexts(values)
	return err
}

// WriteContactFieldTexts stores the values the texts describe on one contact.
func (p *Plugin) WriteContactFieldTexts(
	ctx context.Context, contactID uuid.UUID, values map[string]string,
) error {
	read, err := p.readTexts(values)
	if err != nil {
		return err
	}
	if len(read) == 0 {
		return nil
	}
	return p.store.writeValues(ctx, contactID, read)
}

// readTexts returns the storable values the texts describe, refusing the rest.
func (p *Plugin) readTexts(values map[string]string) (map[string]any, error) {
	live := p.catalog.liveKinds()
	given := make(map[string]any, len(values))
	for name, text := range values {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		given[name] = typedText(live[name], trimmed)
	}
	checked, err := checkValues(live, given)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sdk.ErrInvalidFieldText, err)
	}
	return checked, nil
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
