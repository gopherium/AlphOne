// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// coreSettings returns one valid value for every core setting the server reads, the mail ones legal together.
func coreSettings() map[string]string {
	return map[string]string{
		"ALPHONE_DATABASE_URL":              "postgres://localhost:5433/alphone",
		"ALPHONE_ADDR":                      "127.0.0.1:9000",
		"ALPHONE_WEB_DIR":                   "/srv/alphone/web",
		"ALPHONE_TRUSTED_PROXIES":           "10.0.0.0/8,192.168.0.0/16",
		"ALPHONE_WEBHOOK_ALLOWED_HOSTS":     "127.0.0.1/32,n8n:5678",
		"ALPHONE_DEV_GRAPHIQL":              "1",
		"ALPHONE_TENANT_MACHINE_GRACE":      "72h",
		"ALPHONE_TENANTS_HELD":              "8",
		"ALPHONE_TENANTS_REFRESH":           "30s",
		"ALPHONE_SMTP_HOST":                 "mail.example.com",
		"ALPHONE_SMTP_PORT":                 "2525",
		"ALPHONE_SMTP_USERNAME":             "crm",
		"ALPHONE_SMTP_PASSWORD":             "correct horse battery",
		"ALPHONE_SMTP_FROM":                 "crm@example.com",
		"ALPHONE_SMTP_TLS":                  "opportunistic",
		"ALPHONE_PUBLIC_URL":                "https://crm.example.com",
		"ALPHONE_MAIL_TEMPLATE_DIR":         "/srv/alphone/mail",
		"ALPHONE_INVITE_TTL":                "24h",
		"ALPHONE_RESET_TTL":                 "30m",
		"ALPHONE_RESET_ATTEMPTS":            "5",
		"ALPHONE_RESET_LINKS":               "4",
		"ALPHONE_RESET_COOLDOWN":            "2m",
		"ALPHONE_GRAPH_PAGE_SIZE":           "25",
		"ALPHONE_GRAPH_PAGE_CAP":            "400",
		"ALPHONE_GRAPH_OPERATIONS_PER_USER": "12",
		"ALPHONE_GRAPH_OPERATION_TIMEOUT":   "45s",
		"ALPHONE_GRAPH_BODY_MAX_BYTES":      "524288",
		"ALPHONE_GRAPH_UPLOAD_MAX_BYTES":    "3145728",
		"ALPHONE_GRAPH_RETRY_AFTER":         "2s",
		"ALPHONE_GRAPH_ANONYMOUS_MAX_BYTES": "8192",
		"ALPHONE_GRAPH_ANONYMOUS_PER_IP":    "3",
		"ALPHONE_GRAPH_ANONYMOUS_CEILING":   "12",
		"ALPHONE_STREAMS_PER_USER":          "4",
		"ALPHONE_STREAM_LIFETIME":           "3m",
		"ALPHONE_TOAST_DURATION":            "9s",
		"ALPHONE_LIST_PAGE_SIZES":           "5,15,30",
		"ALPHONE_LIST_PAGE_SIZE":            "15",
		"ALPHONE_FORMAT_LOCALE":             "de-DE",
		"ALPHONE_HTTP_READ_HEADER_TIMEOUT":  "5s",
		"ALPHONE_HTTP_READ_TIMEOUT":         "20s",
		"ALPHONE_HTTP_IDLE_TIMEOUT":         "90s",
		"ALPHONE_SHUTDOWN_GRACE":            "15s",
		"ALPHONE_SHUTDOWN_CANCEL_GRACE":     "3s",
		"ALPHONE_SHUTDOWN_STOP_GRACE":       "4s",
	}
}

// settingsReadElsewhere names the settings of the example file that a plugin or a command reads, not the server.
var settingsReadElsewhere = []string{
	"ALPHONE_COMMAND_RECORD_TIMEOUT",
	"ALPHONE_COMMAND_RECORDS_LIMIT",
	"ALPHONE_TOKEN_TTL_DAYS",
	"ALPHONE_FIELDS_ENTRIES_MAX",
	"ALPHONE_WHATSAPP_VERIFY_TOKEN",
	"ALPHONE_WHATSAPP_APP_SECRET",
	"ALPHONE_WHATSAPP_ACCESS_TOKEN",
	"ALPHONE_WHATSAPP_PHONE_NUMBER_ID",
	"ALPHONE_WHATSAPP_CREDENTIALS_KEY",
	"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES",
	"ALPHONE_WHATSAPP_GRAPH_URL",
}

// exampleSetting matches one setting line of the example file, commented or not, capturing its key and its value.
var exampleSetting = regexp.MustCompile(`^#?\s*(ALPHONE_[A-Z0-9_]+)=(.*)$`)

// exampleSettings returns the value of every ALPHONE_ key the example file names, commented lines included.
func exampleSettings(t *testing.T) map[string]string {
	t.Helper()
	example, err := os.ReadFile(filepath.Join("..", "..", ".env.example"))
	if err != nil {
		t.Fatalf("reading the example file: %v", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(example), "\n") {
		if found := exampleSetting.FindStringSubmatch(line); found != nil {
			values[found[1]] = found[2]
		}
	}
	return values
}

func TestPaddedSettingsLoadLikePlainOnes(t *testing.T) {
	t.Parallel()

	plain := coreSettings()
	padded := map[string]string{}
	for key, value := range plain {
		padded[key] = "  " + value + "  "
	}
	want, err := loadRunConfig(testGetenv(plain))
	if err != nil {
		t.Fatalf("loadRunConfig() over plain values error = %v, want nil", err)
	}

	got, err := loadRunConfig(testGetenv(padded))

	if err != nil {
		t.Fatalf("loadRunConfig() over padded values error = %v, want every value read trimmed", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loadRunConfig() over padded values = %+v, want %+v", got, want)
	}
}

func TestBlankSettingsLoadLikeUnsetOnes(t *testing.T) {
	t.Parallel()

	databaseURL := coreSettings()["ALPHONE_DATABASE_URL"]
	blank := map[string]string{}
	for key := range coreSettings() {
		blank[key] = "   "
	}
	blank["ALPHONE_DATABASE_URL"] = databaseURL
	want, err := loadRunConfig(testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}))
	if err != nil {
		t.Fatalf("loadRunConfig() over the address alone error = %v, want nil", err)
	}

	got, err := loadRunConfig(testGetenv(blank))

	if err != nil {
		t.Fatalf("loadRunConfig() over blank values error = %v, want every blank value taken as unset", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loadRunConfig() over blank values = %+v, want %+v", got, want)
	}
}

func TestTheServerReadsExactlyTheCoreSettings(t *testing.T) {
	t.Parallel()

	values := coreSettings()
	read := map[string]bool{}
	recording := func(key string) string {
		read[key] = true
		return values[key]
	}

	if _, err := loadRunConfig(recording); err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}

	got := slices.Sorted(maps.Keys(read))
	want := slices.Sorted(maps.Keys(values))
	if !slices.Equal(got, want) {
		t.Errorf("loadRunConfig() read %v, want exactly the core settings %v", got, want)
	}
}

func TestEverySettingTheExampleFileNamesHasAnOwner(t *testing.T) {
	t.Parallel()

	named := exampleSettings(t)
	owned := append(slices.Collect(maps.Keys(coreSettings())), settingsReadElsewhere...)

	for key := range named {
		if !slices.Contains(owned, key) {
			t.Errorf("the example file names %s, which nothing reads", key)
		}
	}
	for _, key := range owned {
		if _, held := named[key]; !held {
			t.Errorf("%s is read but the example file never names it", key)
		}
	}
}

func TestTheExampleFileStatesTheCommandFallbacks(t *testing.T) {
	t.Parallel()

	named := exampleSettings(t)
	fallbacks := map[string]string{
		"ALPHONE_COMMAND_RECORD_TIMEOUT": recordTimeout.String(),
		"ALPHONE_COMMAND_RECORDS_LIMIT":  strconv.Itoa(recordsLimit),
		"ALPHONE_TOKEN_TTL_DAYS":         strconv.Itoa(defaultTokenDays),
	}

	for key, want := range fallbacks {
		if named[key] != want {
			t.Errorf("the example file states %s=%q, want the fallback %q", key, named[key], want)
		}
	}
}

func TestSettingRefusalsNameTheSettingAndTheReason(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		key    string
		raw    string
		reason string
	}{
		"a machine grace in words": {"ALPHONE_TENANT_MACHINE_GRACE", "a fortnight", "must be a duration like 30s"},
		"a negative machine grace": {"ALPHONE_TENANT_MACHINE_GRACE", "-1h", "must not be negative"},
		"a held count in words":    {"ALPHONE_TENANTS_HELD", "many", "must be a whole number"},
		"no tenants held":          {"ALPHONE_TENANTS_HELD", "0", "must stand above zero"},
		"no refresh at all":        {"ALPHONE_TENANTS_REFRESH", "0s", "must stand above zero"},
		"a relay port in words":    {"ALPHONE_SMTP_PORT", "the submission port", "must be a whole number"},
		"a relay port too high":    {"ALPHONE_SMTP_PORT", "70000", "must stand at or below 65535"},
		"an unknown transport":     {"ALPHONE_SMTP_TLS", "tls13", "must be mandatory, opportunistic or none"},
		"a public address on ftp":  {"ALPHONE_PUBLIC_URL", "ftp://crm.example.com", "must be an http or https address"},
		"a public address query":   {"ALPHONE_PUBLIC_URL", "https://crm.example.com?a=1", "must carry no query or fragment"},
		"a public address path":    {"ALPHONE_PUBLIC_URL", "https://crm.example.com/app", "must name a site root"},
		"an invite TTL in words":   {"ALPHONE_INVITE_TTL", "a week", "must be a duration like 30s"},
		"no reset lifetime":        {"ALPHONE_RESET_TTL", "0s", "must stand above zero"},
		"negative reset attempts":  {"ALPHONE_RESET_ATTEMPTS", "-1", "must stand above zero"},
		"reset links in words":     {"ALPHONE_RESET_LINKS", "a few", "must be a whole number"},
		"a cooldown in words":      {"ALPHONE_RESET_COOLDOWN", "soon", "must be a duration like 30s"},
		"a graph page in words":    {"ALPHONE_GRAPH_PAGE_SIZE", "fifty", "must be a whole number"},
		"no rows a graph page":     {"ALPHONE_GRAPH_PAGE_SIZE", "0", "must stand above zero"},
		"a graph cap in words":     {"ALPHONE_GRAPH_PAGE_CAP", "many", "must be a whole number"},
		"a toast in words": {
			"ALPHONE_TOAST_DURATION", "a while",
			"must be a duration such as 6s or 1500ms, in whole milliseconds from 1ms to 2147483647ms",
		},
		"list pages in words":      {"ALPHONE_LIST_PAGE_SIZES", "10,twenty", "must list positive whole numbers"},
		"an opening page in words": {"ALPHONE_LIST_PAGE_SIZE", "twenty", "must be a whole number"},
		"a locale in words": {
			"ALPHONE_FORMAT_LOCALE", "not a locale", "must be a BCP 47 language tag such as es-ES or en-GB",
		},
		"a webhook host without a port": {
			"ALPHONE_WEBHOOK_ALLOWED_HOSTS", "127.0.0.1/32,n8n",
			"must list CIDR ranges such as 127.0.0.1/32 or host names with a port such as n8n:5678",
		},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			env := coreSettings()
			env[tt.key] = tt.raw
			want := fmt.Sprintf("%s: %s, got %q", tt.key, tt.reason, tt.raw)

			_, err := loadRunConfig(testGetenv(env))

			if err == nil || err.Error() != want {
				t.Errorf("loadRunConfig() error = %v, want %q", err, want)
			}
		})
	}
}
