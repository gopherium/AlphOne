// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/pluginkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/sdk"
)

// Demo credentials stored by the seed subcommand, for development only.
const (
	seedAdminEmail    = "admin@example.com"
	seedAdminName     = "Admin"
	seedAdminPassword = "password1234"
)

// seed migrates the database and stores the demo data set.
func seed(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	if err := migrateSchemas(ctx, databaseURL); err != nil {
		return err
	}
	created, err := authkit.EnsureAdmin(
		ctx, authkitpg.NewUserStore(pool), seedAdminEmail, seedAdminName, seedAdminPassword)
	if err != nil {
		return err
	}
	if err := grantAdmin(ctx, pool, authkitpg.NewUserStore(pool), seedAdminEmail); err != nil {
		return err
	}
	resolver := contact.NewResolver(postgres.NewContactStore(pool))
	ada, err := resolver.Resolve(ctx, "email", "ada@example.com", "Ada Lovelace")
	if err != nil {
		return fmt.Errorf("seed contact: %w", err)
	}
	tasks := postgres.NewTaskStore(pool)
	if err := seedTasks(ctx, tasks, authkitpg.NewUserStore(pool), ada.ID); err != nil {
		return err
	}
	if err := seedPlugins(ctx, databaseURL, getenv, resolver); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "seeded demo data")
	if created {
		_, _ = fmt.Fprintln(stdout, "login: "+seedAdminEmail+" / "+seedAdminPassword)
	} else {
		_, _ = fmt.Fprintln(stdout, seedAdminEmail+" already exists, its password is unchanged")
	}
	_, _ = fmt.Fprintln(stdout, "development only, never seed a production database")
	return nil
}

// demoTask is one scripted task of the demo data set.
type demoTask struct {
	id       string
	title    string
	offset   int
	priority int
	done     bool
	linked   bool
	origin   string
}

// demoTasks returns the scripted tasks stored by [seedTasks].
func demoTasks() []demoTask {
	return []demoTask{
		{id: "0198d000-0000-7000-8000-000000000001", title: "Chase the overdue invoice", offset: -1},
		{id: "0198d000-0000-7000-8000-000000000002", title: "Call Ada about the renewal", linked: true},
		{id: "0198d000-0000-7000-8000-000000000003", title: "Approve the new pricing", priority: 1},
		{id: "0198d000-0000-7000-8000-000000000004", title: "File the delivery notes", done: true},
		{
			id:     "0198d000-0000-7000-8000-000000000005",
			title:  "Reply to the imported enquiry",
			offset: 1,
			origin: "seed",
		},
	}
}

// seedOriginEvent is the event the scripted task with an origin points at.
const seedOriginEvent = "0198d000-0000-7000-8000-0000000000ff"

// seedTasks stores the demo tasks for the admin account, skipping the ones
// stored by an earlier run.
func seedTasks(
	ctx context.Context,
	store *postgres.TaskStore,
	users gouncer.Store,
	contactID uuid.UUID,
) error {
	admin, err := users.UserByEmail(ctx, seedAdminEmail)
	if err != nil {
		return fmt.Errorf("seed admin lookup: %w", err)
	}
	assigneeID := admin.ID
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, scripted := range demoTasks() {
		id := uuid.MustParse(scripted.id)
		if _, err := store.Get(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, task.ErrNotFound) {
			return fmt.Errorf("seed task lookup: %w", err)
		}
		built, err := buildDemoTask(scripted, id, today, assigneeID, contactID)
		if err != nil {
			return err
		}
		if _, _, err := store.Create(ctx, built); err != nil {
			return err
		}
	}
	return nil
}

// buildDemoTask turns one scripted task into a stored task.
func buildDemoTask(
	scripted demoTask,
	id uuid.UUID,
	today time.Time,
	assigneeID, contactID uuid.UUID,
) (task.Task, error) {
	in := task.Input{
		Title:      scripted.title,
		DueOn:      today.AddDate(0, 0, scripted.offset),
		Priority:   scripted.priority,
		AssigneeID: assigneeID,
	}
	if scripted.linked {
		in.ContactID = contactID
	}
	if scripted.origin != "" {
		in.Origin = task.Origin{Source: scripted.origin, EventID: uuid.MustParse(seedOriginEvent)}
	}
	built, err := task.New(in)
	if err != nil {
		return task.Task{}, fmt.Errorf("build task: %w", err)
	}
	built.ID = id
	if !scripted.done {
		return built, nil
	}
	done := task.StatusDone
	return built.Apply(task.Changes{Status: &done})
}

// seedWhatsApp registers the WhatsApp plugin and stores its demo conversations.
func seedPlugins(
	ctx context.Context,
	databaseURL string,
	getenv func(string) string,
	resolver *contact.Resolver,
) error {
	registered, err := registerPlugins(sdk.Deps{
		DatabaseURL: databaseURL,
		Resolver:    resolverBridge{resolver: resolver},
		Contacts:    directoryBridge{resolver: resolver},
		Getenv:      getenv,
	})
	if err != nil {
		return err
	}
	wireFieldProviders(registered)
	host := pluginkit.NewHost(registered...)
	defer func() { _ = host.Stop(context.WithoutCancel(ctx)) }()
	if err := host.Start(ctx); err != nil {
		return err
	}
	return host.Seed(ctx)
}
