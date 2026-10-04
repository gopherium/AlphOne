// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole"
)

func TestTheCommandLineTypesAreTheFrameworkOnes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ named, framework reflect.Type }{
		"Command":         {reflect.TypeFor[Command](), reflect.TypeFor[gonsole.Command]()},
		"Call":            {reflect.TypeFor[Call](), reflect.TypeFor[gonsole.Call]()},
		"CommandProvider": {reflect.TypeFor[CommandProvider](), reflect.TypeFor[gonsole.Provider]()},
		"Env":             {reflect.TypeFor[Env](), reflect.TypeFor[gonsole.Env]()},
		"Bound":           {reflect.TypeFor[Bound](), reflect.TypeFor[gonsole.Bound]()},
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

// settingOf returns a reader under the program prefix holding only the setting named key with value.
func settingOf(key, value string) Env {
	return Env{Prefix: "ALPHONE_", Getenv: func(name string) string {
		return map[string]string{key: value}[name]
	}}
}

func TestAtMostRefusesAValueAboveItsBound(t *testing.T) {
	t.Parallel()

	env := settingOf("ALPHONE_FIELDS_ENTRIES_MAX", "8")

	_, err := env.Count("FIELDS_ENTRIES_MAX", 500, AtMost(7))

	want := `ALPHONE_FIELDS_ENTRIES_MAX: must stand at or below 7, got "8"`
	if err == nil || err.Error() != want {
		t.Errorf("Count() error = %v, want %q", err, want)
	}
}

func TestAllowZeroAcceptsAZeroSetting(t *testing.T) {
	t.Parallel()

	env := settingOf("ALPHONE_TENANT_MACHINE_GRACE", "0s")

	grace, err := env.Duration("TENANT_MACHINE_GRACE", time.Hour, AllowZero())

	if err != nil || grace != 0 {
		t.Errorf("Duration() = %v, %v, want zero accepted", grace, err)
	}
}

func TestParseNamesTheSettingInARefusal(t *testing.T) {
	t.Parallel()

	env := settingOf("ALPHONE_WHATSAPP_CREDENTIALS_KEY", " not hex ")
	var handed string
	refusing := func(value string) ([]byte, error) {
		handed = value
		return nil, errors.New("must be hex encoded")
	}

	_, err := Parse(env, "WHATSAPP_CREDENTIALS_KEY", nil, refusing)

	if err == nil || err.Error() != "ALPHONE_WHATSAPP_CREDENTIALS_KEY: must be hex encoded" {
		t.Errorf("Parse() error = %v, want the refusal under the setting's name", err)
	}
	if handed != "not hex" {
		t.Errorf("Parse() handed the parser %q, want the value trimmed", handed)
	}
}

func TestParseAnswersTheFallbackForAnEmptySetting(t *testing.T) {
	t.Parallel()

	env := settingOf("ALPHONE_WHATSAPP_GRAPH_URL", "   ")

	held, err := Parse(env, "WHATSAPP_GRAPH_URL", "https://graph.example.com", func(string) (string, error) {
		return "", errors.New("the parser must not run for an empty setting")
	})

	if err != nil || held != "https://graph.example.com" {
		t.Errorf("Parse() = %q, %v, want the fallback", held, err)
	}
}
