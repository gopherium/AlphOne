// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gopherium/framework/gonsole"
)

func TestTheCommandLineTypesAreTheFrameworkOnes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ named, framework reflect.Type }{
		"Command":         {reflect.TypeFor[Command](), reflect.TypeFor[gonsole.Command]()},
		"Call":            {reflect.TypeFor[Call](), reflect.TypeFor[gonsole.Call]()},
		"CommandProvider": {reflect.TypeFor[CommandProvider](), reflect.TypeFor[gonsole.Provider]()},
		"Env":             {reflect.TypeFor[Env](), reflect.TypeFor[gonsole.Env]()},
	}
	for name, tt := range tests {
		if tt.named != tt.framework {
			t.Errorf("%s = %v, want the framework's own %v", name, tt.named, tt.framework)
		}
	}
}

func TestMisuseMarksAMisusedCommandLine(t *testing.T) {
	t.Parallel()

	err := Misuse(errors.New("importer:run takes no arguments"))

	if !errors.Is(err, gonsole.ErrMisused) {
		t.Errorf("Misuse() = %v, want it marked as a misused command line", err)
	}
}

func TestEnvReadsUnderItsPrefix(t *testing.T) {
	t.Parallel()

	env := Env{Prefix: "ALPHONE_", Getenv: func(key string) string {
		return map[string]string{"ALPHONE_FIELDS_ENTRIES_MAX": " 7 "}[key]
	}}

	entries, err := env.Within("FIELDS_").Count("ENTRIES_MAX", 500)

	if err != nil || entries != 7 {
		t.Errorf("Count(ENTRIES_MAX) = %d, %v, want 7 read trimmed under the program prefix", entries, err)
	}
}
