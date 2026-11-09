// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// workspaceMaxBytesSetting is the one setting that takes zero, as no workspace limit.
const workspaceMaxBytesSetting = "ALPHONE_EMAIL_WORKSPACE_MAX_BYTES"

// countKeys lists every setting the plugin reads as a whole number.
var countKeys = []string{
	"ALPHONE_EMAIL_MESSAGE_MAX_BYTES",
	"ALPHONE_EMAIL_MESSAGE_MAX_PARTS",
	"ALPHONE_EMAIL_MESSAGE_MAX_CONTAINERS",
	"ALPHONE_EMAIL_MESSAGE_MAX_FIELD_BYTES",
	"ALPHONE_EMAIL_HEADER_MAX_BYTES",
	"ALPHONE_EMAIL_BODY_MAX_BYTES",
	"ALPHONE_EMAIL_SUBJECT_MAX_BYTES",
	"ALPHONE_EMAIL_NAME_MAX_BYTES",
	"ALPHONE_EMAIL_RECIPIENTS_MAX",
	"ALPHONE_EMAIL_MAILS_IN_MEMORY_MAX",
	"ALPHONE_EMAIL_SNIPPET_LETTERS",
	"ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES",
	workspaceMaxBytesSetting,
	"ALPHONE_EMAIL_EVENT_TEXT_MAX_BYTES",
	"ALPHONE_EMAIL_THREAD_REFERENCES_MAX",
	"ALPHONE_EMAIL_SWEEP_BATCH",
}

// durationKeys lists every setting the plugin reads as a duration.
var durationKeys = []string{
	"ALPHONE_EMAIL_SUBJECT_THREAD_WINDOW",
	"ALPHONE_EMAIL_RETRY_WINDOW",
	"ALPHONE_EMAIL_CLEANUP_TIMEOUT",
	"ALPHONE_EMAIL_SWEEP_INTERVAL",
	"ALPHONE_EMAIL_SWEEP_BUDGET",
	"ALPHONE_EMAIL_ORPHAN_AGE",
}

// distinctValues sets every setting to a value apart from its default and from every other setting's value.
var distinctValues = map[string]string{
	"ALPHONE_EMAIL_MESSAGE_MAX_BYTES":       "300",
	"ALPHONE_EMAIL_MESSAGE_MAX_PARTS":       "50",
	"ALPHONE_EMAIL_MESSAGE_MAX_CONTAINERS":  "10",
	"ALPHONE_EMAIL_MESSAGE_MAX_FIELD_BYTES": "1024",
	"ALPHONE_EMAIL_HEADER_MAX_BYTES":        "65536",
	"ALPHONE_EMAIL_BODY_MAX_BYTES":          "16",
	"ALPHONE_EMAIL_SUBJECT_MAX_BYTES":       "128",
	"ALPHONE_EMAIL_NAME_MAX_BYTES":          "64",
	"ALPHONE_EMAIL_RECIPIENTS_MAX":          "20",
	"ALPHONE_EMAIL_MAILS_IN_MEMORY_MAX":     "2",
	"ALPHONE_EMAIL_SNIPPET_LETTERS":         "80",
	"ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES":    "8",
	workspaceMaxBytesSetting:                "1450",
	"ALPHONE_EMAIL_EVENT_TEXT_MAX_BYTES":    "32",
	"ALPHONE_EMAIL_THREAD_REFERENCES_MAX":   "12",
	"ALPHONE_EMAIL_SWEEP_BATCH":             "3",
	"ALPHONE_EMAIL_SUBJECT_THREAD_WINDOW":   "48h",
	"ALPHONE_EMAIL_RETRY_WINDOW":            "30m",
	"ALPHONE_EMAIL_CLEANUP_TIMEOUT":         "3s",
	"ALPHONE_EMAIL_SWEEP_INTERVAL":          "2s",
	"ALPHONE_EMAIL_SWEEP_BUDGET":            "90s",
	"ALPHONE_EMAIL_ORPHAN_AGE":              "2h",
}

// settingsEnv returns a reader under the program prefix answering only the given variables.
func settingsEnv(held map[string]string) sdk.Env {
	return sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string { return held[name] }}
}

func TestReadSettingsAnswersEveryDefaultWhenNothingIsSet(t *testing.T) {
	t.Parallel()

	got, err := readSettings(sdk.Deps{})

	if err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}
	want := settings{
		parse: message.Limits{
			MaxBytes: 36700160, MaxParts: 200, MaxContainers: 16, MaxFieldBytes: 4096,
			MaxHeaderBytes: 262144, MaxBodyBytes: 4194304,
		},
		caps: mailCaps{
			mailsInMemoryMax: 1, subjectMaxBytes: 2048, nameMaxBytes: 512, recipientsMax: 500,
			snippetLetters: 200, threadReferencesMax: 64, eventTextMaxBytes: 16384,
		},
		files: fileCaps{attachmentMaxBytes: 26214400, workspaceMaxBytes: 5368709120},
		times: timeLimits{subjectThreadWindow: 336 * time.Hour, retryWindow: 6 * time.Hour, cleanupTimeout: 10 * time.Second},
		sweep: sweepSettings{interval: time.Hour, batch: 100, budget: 10 * time.Minute, orphanAge: 24 * time.Hour},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readSettings() = %+v, want the defaults %+v", got, want)
	}
}

func TestReadSettingsHoldsTheValueOfEverySetting(t *testing.T) {
	t.Parallel()

	got, err := readSettings(sdk.Deps{Env: settingsEnv(distinctValues)})

	if err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}
	want := settings{
		parse: message.Limits{
			MaxBytes: 300, MaxParts: 50, MaxContainers: 10, MaxFieldBytes: 1024, MaxHeaderBytes: 65536, MaxBodyBytes: 16,
		},
		caps: mailCaps{
			mailsInMemoryMax: 2, subjectMaxBytes: 128, nameMaxBytes: 64, recipientsMax: 20,
			snippetLetters: 80, threadReferencesMax: 12, eventTextMaxBytes: 32,
		},
		files: fileCaps{attachmentMaxBytes: 8, workspaceMaxBytes: 1450},
		times: timeLimits{
			subjectThreadWindow: 48 * time.Hour, retryWindow: 30 * time.Minute, cleanupTimeout: 3 * time.Second,
		},
		sweep: sweepSettings{interval: 2 * time.Second, batch: 3, budget: 90 * time.Second, orphanAge: 2 * time.Hour},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readSettings() = %+v, want %+v read from each setting", got, want)
	}
}

func TestReadSettingsNamesTheKeyOfAValueThatDoesNotRead(t *testing.T) {
	t.Parallel()

	for _, key := range slices.Concat(countKeys, durationKeys) {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			_, err := readSettings(sdk.Deps{Env: settingsEnv(map[string]string{key: "plenty"})})

			if err == nil || !strings.HasPrefix(err.Error(), key+": must ") {
				t.Errorf("readSettings() over %s=plenty error = %v, want the value refused by its key", key, err)
			}
		})
	}
}

func TestReadSettingsTakesZeroAsNoWorkspaceLimit(t *testing.T) {
	t.Parallel()

	got, err := readSettings(sdk.Deps{Env: settingsEnv(map[string]string{workspaceMaxBytesSetting: "0"})})

	if err != nil {
		t.Fatalf("readSettings() over %s=0 error = %v, want nil", workspaceMaxBytesSetting, err)
	}
	if got.files.workspaceMaxBytes != 0 {
		t.Errorf("workspaceMaxBytes = %d, want 0 for no limit", got.files.workspaceMaxBytes)
	}
}

func TestReadSettingsRefusesANegativeWorkspaceLimit(t *testing.T) {
	t.Parallel()

	_, err := readSettings(sdk.Deps{Env: settingsEnv(map[string]string{workspaceMaxBytesSetting: "-1"})})

	if err == nil || !strings.HasPrefix(err.Error(), workspaceMaxBytesSetting+": must not be negative") {
		t.Errorf("readSettings() over %s=-1 error = %v, want the negative limit refused by its key",
			workspaceMaxBytesSetting, err)
	}
}

func TestReadSettingsRefusesZeroForEveryOtherSetting(t *testing.T) {
	t.Parallel()

	zeros := map[string]string{}
	for _, key := range countKeys {
		zeros[key] = "0"
	}
	for _, key := range durationKeys {
		zeros[key] = "0s"
	}
	delete(zeros, workspaceMaxBytesSetting)

	for key, zero := range zeros {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			_, err := readSettings(sdk.Deps{Env: settingsEnv(map[string]string{key: zero})})

			if err == nil || !strings.HasPrefix(err.Error(), key+": must stand above zero") {
				t.Errorf("readSettings() over %s=%s error = %v, want zero refused by its key", key, zero, err)
			}
		})
	}
}

func TestReadSettingsReadsExactlyTheEmailSettings(t *testing.T) {
	t.Parallel()

	var read []string
	env := sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string {
		read = append(read, name)
		return ""
	}}

	if _, err := readSettings(sdk.Deps{Env: env}); err != nil {
		t.Fatalf("readSettings() error = %v, want nil", err)
	}

	want := slices.Sorted(maps.Keys(distinctValues))
	if got := slices.Compact(slices.Sorted(slices.Values(read))); !slices.Equal(got, want) {
		t.Errorf("readSettings() read %v, want exactly %v", got, want)
	}
}
