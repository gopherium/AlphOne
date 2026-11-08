// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// filesFeature is the feature the file stores answer to.
const filesFeature = "../../test/features/features/files.feature"

// storedObjects counts the files the storage schema holds, live and unfinished alike.
const storedObjects = "SELECT count(*) FROM storage.objects"

// filesScenario is what one file scenario keeps between its steps.
type filesScenario struct {
	t          *testing.T
	env        map[string]string
	pool       *pgxpool.Pool
	workspaces map[string]uuid.UUID
	keeping    *fileTakingPlugin
	built      composed
	composed   bool
	file       sdk.StoredFile
	chosen     uuid.UUID
	saveErr    error
	serverLog  strings.Builder
	pluginLog  *slog.Logger
}

// initializeFiles returns the binding of the file steps, t holding their databases.
func initializeFiles(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &filesScenario{t: t, env: map[string]string{}, workspaces: map[string]uuid.UUID{}}
		sc.Given(`^the settings point at a migrated database$`, s.pointAtAMigratedDatabase)
		sc.Given(`^the workspace "([^"]*)" exists$`, s.createWorkspace)
		sc.Given(`^the files hold at most (\d+) bytes$`, s.capFiles)
		sc.Given(`^AlphOne runs a plugin that keeps files$`, s.runAPluginThatKeepsFiles)
		sc.When(`^the workspace "([^"]*)" saves the file "([^"]*)"$`, s.save)
		sc.When(`^the workspace "([^"]*)" saves a file of (\d+) bytes$`, s.saveOfSize)
		sc.Given(`^the workspace "([^"]*)" saved the file "([^"]*)" under an id it chose$`, s.saveUnderAChosenID)
		sc.When(`^the workspace "([^"]*)" saves the file "([^"]*)" under an id it chose$`, s.saveUnderAChosenID)
		sc.When(`^the workspace "([^"]*)" saves the file "([^"]*)" under the same id$`, s.saveUnderTheSameID)
		sc.When(`^the plugin reports the failure "([^"]*)"$`, s.reportFailure)
		sc.Then(`^the workspace "([^"]*)" cannot open that file$`, s.cannotOpen)
		sc.Then(`^the workspace "([^"]*)" cannot delete that file$`, s.cannotDelete)
		sc.Then(`^the workspace "([^"]*)" still opens that file as "([^"]*)"$`, s.opens)
		sc.Then(`^the file is stored under the id the plugin chose$`, s.storedUnderTheChosenID)
		sc.Then(`^the save is refused as too large$`, s.refusedAsTooLarge)
		sc.Then(`^the save is refused as an id in use$`, s.refusedAsInUse)
		sc.Then(`^no file is stored$`, s.storesNoFile)
		sc.Then(`^the server log holds the error "([^"]*)"$`, s.logHoldsTheError)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, s.stop(ctx)
		})
	}
}

// pointAtAMigratedDatabase points the settings at a fresh database every schema step migrated.
func (s *filesScenario) pointAtAMigratedDatabase(ctx context.Context) error {
	databaseURL := barePostgres(s.t)
	if err := migrate(ctx, databaseURL); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	s.env["ALPHONE_DATABASE_URL"], s.pool = databaseURL, pool
	return nil
}

// createWorkspace stores the workspace called name and keeps its id.
func (s *filesScenario) createWorkspace(ctx context.Context, name string) error {
	id := uuid.Must(uuid.NewV7())
	if _, err := s.pool.Exec(ctx, "INSERT INTO core.tenants (id, name) VALUES ($1, $2)", id, name); err != nil {
		return fmt.Errorf("storing the workspace %s: %w", name, err)
	}
	s.workspaces[name] = id
	return nil
}

// capFiles holds every file to the most bytes the settings name.
func (s *filesScenario) capFiles(most int) {
	s.env["ALPHONE_STORAGE_MAX_BYTES"] = strconv.Itoa(most)
}

// runAPluginThatKeepsFiles composes the host over the settings and a server log with one plugin taking a file store.
func (s *filesScenario) runAPluginThatKeepsFiles(ctx context.Context) error {
	settings, err := loadRunConfig(testkit.Getenv(s.env))
	if err != nil {
		return err
	}
	s.keeping = &fileTakingPlugin{inertPlugin: inertPlugin{id: "notes"}}
	s.built, err = compose(ctx, composeConfig{
		composeSettings: settings.composeSettings,
		databaseURL:     settings.databaseURL,
		getenv:          testkit.Getenv(s.env),
		roles:           role.NewRegistry(),
		logger:          slog.New(slog.NewTextHandler(&s.serverLog, nil)),
	}, func(deps sdk.Deps) ([]sdk.Plugin, error) {
		s.pluginLog = deps.Logger
		return []sdk.Plugin{s.keeping}, nil
	})
	s.composed = true
	if err != nil {
		return err
	}
	return s.built.files.Check(ctx)
}

// in returns ctx serving the workspace called name.
func (s *filesScenario) in(ctx context.Context, name string) context.Context {
	return sdk.WithTenant(ctx, s.workspaces[name])
}

// save saves the text as a file of the workspace called name and keeps what the store answered.
func (s *filesScenario) save(ctx context.Context, name, text string) error {
	s.file, s.saveErr = s.keeping.received.Save(s.in(ctx, name), "text/plain", strings.NewReader(text))
	return s.saveErr
}

// saveOfSize tries to save a file of size bytes for the workspace called name.
func (s *filesScenario) saveOfSize(ctx context.Context, name string, size int) {
	body := strings.NewReader(strings.Repeat("x", size))
	s.file, s.saveErr = s.keeping.received.Save(s.in(ctx, name), "text/plain", body)
}

// saveUnderAChosenID saves the text as a file of the workspace called name under a new id the plugin chose.
func (s *filesScenario) saveUnderAChosenID(ctx context.Context, name, text string) error {
	s.chosen = uuid.Must(uuid.NewV7())
	s.file, s.saveErr = s.keeping.received.SaveAs(s.in(ctx, name), s.chosen, "text/plain", strings.NewReader(text))
	return s.saveErr
}

// saveUnderTheSameID tries to save the text for the workspace called name under the id the plugin chose before.
func (s *filesScenario) saveUnderTheSameID(ctx context.Context, name, text string) {
	_, s.saveErr = s.keeping.received.SaveAs(s.in(ctx, name), s.chosen, "text/plain", strings.NewReader(text))
}

// storedUnderTheChosenID fails unless the store answered the id the plugin chose.
func (s *filesScenario) storedUnderTheChosenID() error {
	if s.file.ID != s.chosen {
		return fmt.Errorf("SaveAs() answered the id %v, want the chosen %v", s.file.ID, s.chosen)
	}
	return nil
}

// cannotOpen fails unless the workspace called name finds no file under the saved id.
func (s *filesScenario) cannotOpen(ctx context.Context, name string) error {
	if _, err := s.keeping.received.Open(s.in(ctx, name), s.file.ID); !errors.Is(err, sdk.ErrFileNotFound) {
		return fmt.Errorf("Open() from %s error = %v, want %v", name, err, sdk.ErrFileNotFound)
	}
	return nil
}

// cannotDelete fails unless the workspace called name finds no file under the saved id to delete.
func (s *filesScenario) cannotDelete(ctx context.Context, name string) error {
	if err := s.keeping.received.Delete(s.in(ctx, name), s.file.ID); !errors.Is(err, sdk.ErrFileNotFound) {
		return fmt.Errorf("Delete() from %s error = %v, want %v", name, err, sdk.ErrFileNotFound)
	}
	return nil
}

// opens fails unless the workspace called name opens the saved file holding the text.
func (s *filesScenario) opens(ctx context.Context, name, text string) error {
	reader, err := s.keeping.received.Open(s.in(ctx, name), s.file.ID)
	if err != nil {
		return fmt.Errorf("Open() from %s error = %v, want the file", name, err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != text {
		return fmt.Errorf("Open() from %s read %q, %v, want %q", name, body, err, text)
	}
	return nil
}

// refusedAsTooLarge fails unless the last save answered that the file is too large.
func (s *filesScenario) refusedAsTooLarge() error {
	if !errors.Is(s.saveErr, sdk.ErrFileTooLarge) {
		return fmt.Errorf("Save() error = %v, want %v", s.saveErr, sdk.ErrFileTooLarge)
	}
	return nil
}

// refusedAsInUse fails unless the last save answered that the id already holds a file.
func (s *filesScenario) refusedAsInUse() error {
	if !errors.Is(s.saveErr, sdk.ErrFileExists) {
		return fmt.Errorf("SaveAs() error = %v, want %v", s.saveErr, sdk.ErrFileExists)
	}
	return nil
}

// reportFailure writes text as an error through the logger the plugin was handed.
func (s *filesScenario) reportFailure(text string) error {
	if s.pluginLog == nil {
		return errors.New("the plugin received no logger, want the host's")
	}
	s.pluginLog.Error(text)
	return nil
}

// logHoldsTheError fails unless the server log holds text as an error.
func (s *filesScenario) logHoldsTheError(text string) error {
	if want := "level=ERROR msg=" + strconv.Quote(text); !strings.Contains(s.serverLog.String(), want) {
		return fmt.Errorf("the server log holds %q, want the line %s", s.serverLog.String(), want)
	}
	return nil
}

// storesNoFile fails unless the storage schema holds no file at all.
func (s *filesScenario) storesNoFile(ctx context.Context) error {
	var stored int
	if err := s.pool.QueryRow(ctx, storedObjects).Scan(&stored); err != nil || stored != 0 {
		return fmt.Errorf("the storage schema holds %d files (%v), want none", stored, err)
	}
	return nil
}

// stop stops what the scenario composed and closes its pool.
func (s *filesScenario) stop(ctx context.Context) error {
	var err error
	if s.composed {
		err = abandon(ctx, s.built, pluginStopGrace)
	}
	if s.pool != nil {
		s.pool.Close()
	}
	return err
}

func TestFiles(t *testing.T) {
	runFeature(t, filesFeature, initializeFiles(t))
}
