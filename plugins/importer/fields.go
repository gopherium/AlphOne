// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// fieldName is a contact detail a column may be mapped onto.
type fieldName string

// The fields an import may fill.
const (
	fieldContactName fieldName = "name"
	fieldEmail       fieldName = "email"
	fieldPhone       fieldName = "phone"
)

// mappableField is one field of the registry, as the graph reports it.
type mappableField struct {
	Name     fieldName
	Label    string
	Required bool
}

// coreFields lists the columns the importer serves without any provider.
var coreFields = []mappableField{
	{Name: fieldContactName, Label: "Name", Required: true},
	{Name: fieldEmail, Label: "Email", Required: false},
	{Name: fieldPhone, Label: "Phone", Required: false},
}

// core reports whether the name is one of the importer's own columns.
func core(name fieldName) bool {
	return slices.ContainsFunc(coreFields, func(field mappableField) bool {
		return field.Name == name
	})
}

// registry is the mappable registry beside the provider serving each field.
type registry struct {
	fields    []mappableField
	owner     map[fieldName]int
	providers []sdk.FieldProvider
}

// UseFieldProviders receives the providers serving runtime defined fields.
func (p *Plugin) UseFieldProviders(providers []sdk.FieldProvider) {
	p.providers = providers
}

// registry merges the core columns with every field the providers serve.
func (p *Plugin) registry(ctx context.Context) (registry, error) {
	held := registry{
		fields:    slices.Clone(coreFields),
		owner:     map[fieldName]int{},
		providers: p.providers,
	}
	for index, provider := range p.providers {
		listed, err := provider.LiveContactFields(ctx)
		if err != nil {
			return registry{}, err
		}
		for _, field := range listed {
			held.add(fieldName(field.Name), field.Label, index)
		}
	}
	return held, nil
}

// add lists one provider served field unless its name is already claimed.
func (r *registry) add(name fieldName, label string, index int) {
	if core(name) {
		return
	}
	if _, claimed := r.owner[name]; claimed {
		return
	}
	r.owner[name] = index
	r.fields = append(r.fields, mappableField{Name: name, Label: label})
}

// holds reports whether the registry carries the named field.
func (r registry) holds(name fieldName) bool {
	if core(name) {
		return true
	}
	_, served := r.owner[name]
	return served
}

// shares is one row's field texts, keyed by the provider serving each field.
type shares map[int]map[string]string

// group sorts the texts by the provider serving each field, refusing the unserved.
func (r registry) group(texts map[string]string) (shares, error) {
	grouped := make(shares, len(texts))
	var unserved []string
	for name, text := range texts {
		index, served := r.owner[fieldName(name)]
		if !served {
			unserved = append(unserved, name)
			continue
		}
		if grouped[index] == nil {
			grouped[index] = map[string]string{}
		}
		grouped[index][name] = text
	}
	if len(unserved) > 0 {
		slices.Sort(unserved)
		return nil, fmt.Errorf("%w: %w", sdk.ErrInvalidFieldText,
			fmt.Errorf("no live field holds %s", strings.Join(unserved, ", ")))
	}
	return grouped, nil
}

// check reports whether every provider accepts the share its own fields carry.
func (r registry) check(ctx context.Context, grouped shares) error {
	for index, held := range grouped {
		if err := r.providers[index].CheckContactFieldTexts(ctx, held); err != nil {
			return err
		}
	}
	return nil
}

// write stores every share through the provider serving it.
func (r registry) write(ctx context.Context, contactID uuid.UUID, grouped shares) error {
	for index, held := range grouped {
		if err := r.providers[index].WriteContactFieldTexts(ctx, contactID, held); err != nil {
			return err
		}
	}
	return nil
}

// writeOne stores one text through the provider serving its field, if any does.
func (r registry) writeOne(ctx context.Context, contactID uuid.UUID, name, text string) error {
	index, served := r.owner[fieldName(name)]
	if !served {
		return nil
	}
	return r.providers[index].WriteContactFieldTexts(ctx, contactID, map[string]string{name: text})
}
