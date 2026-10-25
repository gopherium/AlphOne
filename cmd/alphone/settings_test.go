// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole/testkit"
)

// coreSettings returns one valid value for every core setting the server reads, the mail ones legal together.
func coreSettings() map[string]string {
	return map[string]string{
		"ALPHONE_DATABASE_URL":                     "postgres://localhost:5433/alphone",
		"ALPHONE_ADDR":                             "127.0.0.1:9000",
		"ALPHONE_WEB_DIR":                          "/srv/alphone/web",
		"ALPHONE_TRUSTED_PROXIES":                  "10.0.0.0/8,192.168.0.0/16",
		"ALPHONE_WEBHOOK_ALLOWED_HOSTS":            "127.0.0.1/32,n8n:5678",
		"ALPHONE_DEV_GRAPHIQL":                     "1",
		"ALPHONE_TENANT_MACHINE_GRACE":             "72h",
		"ALPHONE_TENANTS_HELD":                     "8",
		"ALPHONE_TENANTS_REFRESH":                  "30s",
		"ALPHONE_CONTACT_MATCH_MAX":                "40",
		"ALPHONE_SMTP_HOST":                        "mail.example.com",
		"ALPHONE_SMTP_PORT":                        "2525",
		"ALPHONE_SMTP_USERNAME":                    "crm",
		"ALPHONE_SMTP_PASSWORD":                    "correct horse battery",
		"ALPHONE_SMTP_FROM":                        "crm@example.com",
		"ALPHONE_SMTP_TLS":                         "opportunistic",
		"ALPHONE_PUBLIC_URL":                       "https://crm.example.com",
		"ALPHONE_MAIL_TEMPLATE_DIR":                "/srv/alphone/mail",
		"ALPHONE_INVITE_TTL":                       "24h",
		"ALPHONE_RESET_TTL":                        "30m",
		"ALPHONE_RESET_ATTEMPTS":                   "5",
		"ALPHONE_RESET_LINKS":                      "4",
		"ALPHONE_RESET_COOLDOWN":                   "2m",
		"ALPHONE_GRAPH_PAGE_SIZE":                  "25",
		"ALPHONE_GRAPH_PAGE_CAP":                   "400",
		"ALPHONE_GRAPH_OPERATIONS_PER_USER":        "12",
		"ALPHONE_GRAPH_OPERATION_TIMEOUT":          "45s",
		"ALPHONE_GRAPH_BODY_MAX_BYTES":             "524288",
		"ALPHONE_GRAPH_UPLOAD_MAX_BYTES":           "3145728",
		"ALPHONE_GRAPH_RETRY_AFTER":                "2s",
		"ALPHONE_GRAPH_ANONYMOUS_MAX_BYTES":        "8192",
		"ALPHONE_GRAPH_ANONYMOUS_PER_IP":           "3",
		"ALPHONE_GRAPH_ANONYMOUS_CEILING":          "12",
		"ALPHONE_GRAPH_ANONYMOUS_MAX_TOKENS":       "48",
		"ALPHONE_GRAPH_ANONYMOUS_QUERY_MAX_BYTES":  "768",
		"ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES": "12288",
		"ALPHONE_STREAMS_PER_USER":                 "4",
		"ALPHONE_STREAM_LIFETIME":                  "3m",
		"ALPHONE_TOAST_DURATION":                   "9s",
		"ALPHONE_LIST_PAGE_SIZES":                  "5,15,30",
		"ALPHONE_LIST_PAGE_SIZE":                   "15",
		"ALPHONE_FORMAT_LOCALE":                    "de-DE",
		"ALPHONE_HTTP_READ_HEADER_TIMEOUT":         "5s",
		"ALPHONE_HTTP_READ_TIMEOUT":                "20s",
		"ALPHONE_HTTP_IDLE_TIMEOUT":                "90s",
		"ALPHONE_SHUTDOWN_GRACE":                   "15s",
		"ALPHONE_SHUTDOWN_CANCEL_GRACE":            "3s",
		"ALPHONE_SHUTDOWN_STOP_GRACE":              "4s",
	}
}

// settingsReadElsewhere names the example file settings that a command or the Go tests read, not the server.
var settingsReadElsewhere = []string{
	"ALPHONE_TEST_DATABASE_URL",
	"ALPHONE_TEST_DATABASE_MAX_AGE",
	"ALPHONE_COMMAND_RECORD_TIMEOUT",
	"ALPHONE_COMMAND_RECORDS_LIMIT",
	"ALPHONE_TOKEN_TTL_DAYS",
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

// settingOwner is the ALPHONE_<ID>_ prefix of one plugin's settings beside the folder holding the plugin.
type settingOwner struct {
	prefix string
	dir    string
}

// pluginSettingOwners returns the setting owner of every plugin under each plugin root of a tree.
func pluginSettingOwners(t *testing.T, tree string) []settingOwner {
	t.Helper()
	var owners []settingOwner
	for _, root := range []string{"plugins", "enterprise"} {
		paths, err := filepath.Glob(filepath.Join(tree, root, "*", "plugin.json"))
		if err != nil {
			t.Fatalf("globbing the manifests under %s: %v", root, err)
		}
		for _, path := range paths {
			owners = append(owners, settingOwner{prefix: settingPrefix(t, path), dir: filepath.Dir(path)})
		}
	}
	return owners
}

// goFiles returns every Go file under a folder parsed, tests and node_modules left out.
func goFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	var parsed []*ast.File
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir() && entry.Name() == "node_modules":
			return filepath.SkipDir
		case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			parsed = append(parsed, file)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return parsed
}

// goStrings returns the text of every string literal the Go files under a folder spell, tests left out.
func goStrings(t *testing.T, dir string) []string {
	t.Helper()
	var spelled []string
	for _, file := range goFiles(t, dir) {
		ast.Inspect(file, func(node ast.Node) bool {
			if text, ok := stringLiteral(node); ok {
				spelled = append(spelled, text)
			}
			return true
		})
	}
	return spelled
}

// stringLiteral returns the text one string literal spells, and whether the node is one.
func stringLiteral(node ast.Node) (string, bool) {
	literal, ok := node.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(literal.Value)
	return text, err == nil
}

// envReaders names the methods through which an environment value reads one setting.
var envReaders = []string{"Value", "Required", "Duration", "Count", "Counts", "Flag"}

// settingName matches a literal spelling a setting name, a trailing underscore marking a prefix instead.
var settingName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*[A-Z0-9]$`)

// settingsRead returns every key the Go files of one plugin read through an environment value, tests left out.
func settingsRead(t *testing.T, owner settingOwner) []string {
	t.Helper()
	files := goFiles(t, owner.dir)
	consts := stringValues(files)
	var keys []string
	for _, file := range files {
		for _, declared := range file.Decls {
			if function, ok := declared.(*ast.FuncDecl); ok && function.Body != nil {
				keys = append(keys, functionReads(function.Body, consts)...)
			}
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// functionReads returns every key one function body reads through an environment value, following the names it binds.
func functionReads(body *ast.BlockStmt, consts map[string]string) []string {
	bound := map[string]string{}
	var keys []string
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			bindEnvironments(node, bound, consts)
		case *ast.CallExpr:
			if key, ok := readKey(node, bound, consts); ok {
				keys = append(keys, key)
			}
		}
		return true
	})
	return keys
}

// bindEnvironments records each name one assignment binds to an environment value, beside the prefix it reads under.
func bindEnvironments(assign *ast.AssignStmt, bound, consts map[string]string) {
	if len(assign.Lhs) != len(assign.Rhs) {
		return
	}
	for i, target := range assign.Lhs {
		name, isName := target.(*ast.Ident)
		if prefix, ok := envPrefix(assign.Rhs[i], bound, consts); isName && ok {
			bound[name.Name] = prefix
		}
	}
}

// envPrefix returns the prefix the environment value of an expression reads under, and whether it is one.
func envPrefix(expr ast.Expr, bound, consts map[string]string) (string, bool) {
	switch expr := expr.(type) {
	case *ast.Ident:
		prefix, ok := bound[expr.Name]
		return prefix, ok
	case *ast.SelectorExpr:
		return "", expr.Sel.Name == "Env"
	case *ast.CallExpr:
		return withinPrefix(expr, bound, consts)
	}
	return "", false
}

// withinPrefix returns the prefix a Within call narrows its environment value to, and whether the call is one.
func withinPrefix(call *ast.CallExpr, bound, consts map[string]string) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) != 1 {
		return "", false
	}
	if selector.Sel.Name != "Within" {
		return "", false
	}
	outer, isEnv := envPrefix(selector.X, bound, consts)
	more, isText := spelled(call.Args[0], consts)
	return outer + more, isEnv && isText
}

// readKey returns the key one call reads through an environment value, and whether it reads one.
func readKey(call *ast.CallExpr, bound, consts map[string]string) (string, bool) {
	receiver, at := readReceiver(call)
	if receiver == nil || len(call.Args) <= at {
		return "", false
	}
	prefix, isEnv := envPrefix(receiver, bound, consts)
	name, isText := spelled(call.Args[at], consts)
	return "ALPHONE_" + prefix + name, isEnv && isText && settingName.MatchString(name)
}

// readReceiver returns the environment expression one reading call goes through beside the index of the name it passes.
func readReceiver(call *ast.CallExpr) (ast.Expr, int) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	switch {
	case !ok:
		return nil, 0
	case selector.Sel.Name == "Parse" && len(call.Args) > 0:
		return call.Args[0], 1
	case slices.Contains(envReaders, selector.Sel.Name):
		return selector.X, 0
	}
	return nil, 0
}

// spelled returns the text a string literal or a string const names, and whether the expression is either.
func spelled(expr ast.Expr, consts map[string]string) (string, bool) {
	if ident, ok := expr.(*ast.Ident); ok {
		text, held := consts[ident.Name]
		return text, held
	}
	return stringLiteral(expr)
}

// stringValues returns the text every single-valued string const or var of the parsed files spells, by name.
func stringValues(files []*ast.File) map[string]string {
	held := map[string]string{}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.ValueSpec); ok {
				recordStrings(spec, held)
			}
			return true
		})
	}
	return held
}

// recordStrings adds the text each name of one value spec spells when it holds a string literal.
func recordStrings(spec *ast.ValueSpec, held map[string]string) {
	if len(spec.Names) != len(spec.Values) {
		return
	}
	for i, name := range spec.Names {
		if text, ok := stringLiteral(spec.Values[i]); ok {
			held[name.Name] = text
		}
	}
}

// unnamedSettings returns every setting the plugins read that the example file never names.
func unnamedSettings(t *testing.T, owners []settingOwner, named map[string]string) []string {
	t.Helper()
	var missing []string
	for _, owner := range owners {
		for _, key := range settingsRead(t, owner) {
			if _, held := named[key]; !held {
				missing = append(missing, key)
			}
		}
	}
	return missing
}

// readsSetting reports whether a Go file of the plugin, tests aside, spells the key past the program or plugin prefix.
func readsSetting(t *testing.T, owner settingOwner, key string) bool {
	t.Helper()
	spelled := goStrings(t, owner.dir)
	return slices.Contains(spelled, strings.TrimPrefix(key, "ALPHONE_")) ||
		slices.Contains(spelled, strings.TrimPrefix(key, owner.prefix))
}

// settingPrefix returns the prefix of the settings the plugin one manifest describes owns.
func settingPrefix(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return "ALPHONE_" + strings.ToUpper(strings.ReplaceAll(manifest.ID, "-", "_")) + "_"
}

// ownerOf returns the plugin whose prefix a setting sits under, and whether one does.
func ownerOf(key string, owners []settingOwner) (settingOwner, bool) {
	at := slices.IndexFunc(owners, func(owner settingOwner) bool { return strings.HasPrefix(key, owner.prefix) })
	if at < 0 {
		return settingOwner{}, false
	}
	return owners[at], true
}

// writeSettingTree writes each file of a temporary tree, keyed by its path inside the tree.
func writeSettingTree(t *testing.T, tree string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Join(tree, filepath.Dir(path)), 0o750); err != nil {
			t.Fatalf("creating the folder of %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(tree, path), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
}

func TestEveryPluginOwnsTheSettingsUnderItsID(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	writeSettingTree(t, tree, map[string]string{
		filepath.Join("plugins", "demo-notes", "plugin.json"): `{"id": "demo-notes"}`,
		filepath.Join("enterprise", "extra", "plugin.json"):   `{"id": "extra"}`,
	})

	got := pluginSettingOwners(t, tree)

	want := []settingOwner{
		{prefix: "ALPHONE_DEMO_NOTES_", dir: filepath.Join(tree, "plugins", "demo-notes")},
		{prefix: "ALPHONE_EXTRA_", dir: filepath.Join(tree, "enterprise", "extra")},
	}
	if !slices.Equal(got, want) {
		t.Errorf("pluginSettingOwners() = %+v, want %+v", got, want)
	}
}

func TestAPluginReadsTheSettingsItsGoCodeSpells(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	writeSettingTree(t, tree, map[string]string{
		filepath.Join("plugins", "demo-notes", "plugin.json"): `{"id": "demo-notes"}`,
		filepath.Join("plugins", "demo-notes", "notes.go"): "package notes\n\n" +
			"const limitSetting = \"DEMO_NOTES_LIMIT\"\n\nvar within = []string{`MAX`}\n",
		filepath.Join("plugins", "demo-notes", "notes_test.go"): "package notes\n\nconst probe = \"PROBE\"\n",
	})
	owner := settingOwner{prefix: "ALPHONE_DEMO_NOTES_", dir: filepath.Join(tree, "plugins", "demo-notes")}

	for key, want := range map[string]bool{
		"ALPHONE_DEMO_NOTES_LIMIT": true,
		"ALPHONE_DEMO_NOTES_MAX":   true,
		"ALPHONE_DEMO_NOTES_PROBE": false,
		"ALPHONE_DEMO_NOTES_GONE":  false,
	} {
		if got := readsSetting(t, owner, key); got != want {
			t.Errorf("readsSetting(%s) = %t, want %t", key, got, want)
		}
	}
}

func TestAPluginReadsTheSettingsItAsksItsEnvironmentFor(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	writeSettingTree(t, tree, map[string]string{
		filepath.Join("plugins", "demo-notes", "plugin.json"): `{"id": "demo-notes"}`,
		filepath.Join("plugins", "demo-notes", "notes.go"): "package notes\n\n" +
			"const limitSetting = \"DEMO_NOTES_LIMIT\"\n\n" +
			"func read() {\n" +
			"\tdeps.Env.Count(limitSetting, 3)\n" +
			"\tenv := deps.Env.Within(\"DEMO_NOTES_\")\n" +
			"\tenv.Value(\"MAX\")\n" +
			"\tsdk.Parse(env, \"SECRET\", nil, nil)\n" +
			"\tbytes.Count(body, \"NOT_A_SETTING\")\n" +
			"\tstrings.Count(\"HELLO\", \"L\")\n" +
			"\tsdk.Parse(other, \"STRAY\", nil, nil)\n" +
			"}\n",
		filepath.Join("plugins", "demo-notes", "notes_test.go"): "package notes\n\n" +
			"func probe() { env.Value(\"PROBE\") }\n",
	})
	owner := settingOwner{prefix: "ALPHONE_DEMO_NOTES_", dir: filepath.Join(tree, "plugins", "demo-notes")}

	got := settingsRead(t, owner)

	want := []string{"ALPHONE_DEMO_NOTES_LIMIT", "ALPHONE_DEMO_NOTES_MAX", "ALPHONE_DEMO_NOTES_SECRET"}
	if !slices.Equal(got, want) {
		t.Errorf("settingsRead() = %v, want %v", got, want)
	}
}

func TestAPluginSettingTheExampleFileLeavesOutIsReported(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	writeSettingTree(t, tree, map[string]string{
		filepath.Join("plugins", "demo-notes", "plugin.json"): `{"id": "demo-notes"}`,
		filepath.Join("plugins", "demo-notes", "notes.go"): "package notes\n\n" +
			"func read() {\n" +
			"\tenv := deps.Env.Within(\"DEMO_NOTES_\")\n" +
			"\tenv.Value(\"LIMIT\")\n" +
			"\tenv.Value(\"KEY\")\n" +
			"\tenv.Value(\"DATABASE_URL\")\n" +
			"\tdeps.Env.Value(\"INVITE_TTL\")\n" +
			"}\n",
	})
	named := map[string]string{
		"ALPHONE_DEMO_NOTES_LIMIT": "5",
		"ALPHONE_DATABASE_URL":     "postgres://example.com/alphone",
		"ALPHONE_INVITE_TTL":       "72h",
	}

	got := unnamedSettings(t, pluginSettingOwners(t, tree), named)

	if want := []string{"ALPHONE_DEMO_NOTES_DATABASE_URL", "ALPHONE_DEMO_NOTES_KEY"}; !slices.Equal(got, want) {
		t.Errorf("unnamedSettings() = %v, want %v", got, want)
	}
}

func TestPaddedSettingsLoadLikePlainOnes(t *testing.T) {
	t.Parallel()

	plain := coreSettings()
	padded := map[string]string{}
	for key, value := range plain {
		padded[key] = "  " + value + "  "
	}
	want, err := loadRunConfig(testkit.Getenv(plain))
	if err != nil {
		t.Fatalf("loadRunConfig() over plain values error = %v, want nil", err)
	}

	got, err := loadRunConfig(testkit.Getenv(padded))

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
	want, err := loadRunConfig(testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}))
	if err != nil {
		t.Fatalf("loadRunConfig() over the address alone error = %v, want nil", err)
	}

	got, err := loadRunConfig(testkit.Getenv(blank))

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
	owners := pluginSettingOwners(t, filepath.Join("..", ".."))

	for key := range named {
		if slices.Contains(owned, key) {
			continue
		}
		owner, found := ownerOf(key, owners)
		switch {
		case !found:
			t.Errorf("the example file names %s, which neither the core nor a plugin reads", key)
		case !readsSetting(t, owner, key):
			t.Errorf("the example file names %s, which no Go file of %s spells", key, owner.dir)
		}
	}
	for _, key := range owned {
		if _, held := named[key]; !held {
			t.Errorf("%s is read but the example file never names it", key)
		}
	}
	public := slices.DeleteFunc(slices.Clone(owners), func(owner settingOwner) bool {
		return !strings.HasPrefix(owner.dir, filepath.Join("..", "..", "plugins"))
	})
	for _, key := range unnamedSettings(t, public, named) {
		t.Errorf("a plugin reads %s but the example file never names it", key)
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
		"a match bound in words":   {"ALPHONE_CONTACT_MATCH_MAX", "a hundred", "must be a whole number"},
		"a match of no identifier": {"ALPHONE_CONTACT_MATCH_MAX", "0", "must stand above zero"},
		"a match past a query row bound": {
			"ALPHONE_CONTACT_MATCH_MAX", "2147483648", "must stand at or below 2147483647",
		},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			env := coreSettings()
			env[tt.key] = tt.raw
			want := fmt.Sprintf("%s: %s, got %q", tt.key, tt.reason, tt.raw)

			_, err := loadRunConfig(testkit.Getenv(env))

			if err == nil || err.Error() != want {
				t.Errorf("loadRunConfig() error = %v, want %q", err, want)
			}
		})
	}
}
