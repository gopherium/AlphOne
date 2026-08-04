// SPDX-License-Identifier: Elastic-2.0

package importer

import "net/http"

// fieldName is a contact detail a column may be mapped onto.
type fieldName string

// The fields an import may fill.
const (
	fieldContactName fieldName = "name"
	fieldEmail       fieldName = "email"
	fieldPhone       fieldName = "phone"
)

type fieldResponse struct {
	Name     fieldName `json:"name"`
	Label    string    `json:"label"`
	Required bool      `json:"required"`
}

// mappableFields lists every field a column may be mapped onto.
var mappableFields = []fieldResponse{
	{Name: fieldContactName, Label: "Name", Required: true},
	{Name: fieldEmail, Label: "Email", Required: false},
	{Name: fieldPhone, Label: "Phone", Required: false},
}

// handleFields returns a handler serving the fields a column may be mapped onto.
func (p *Plugin) handleFields() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, mappableFields)
	}
}

// mappable reports whether the registry carries the named field.
func mappable(name fieldName) bool {
	for _, field := range mappableFields {
		if field.Name == name {
			return true
		}
	}
	return false
}
