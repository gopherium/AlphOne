// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/pluginkit"
	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/sdk"
)

// Demo credentials stored by the seed subcommand, for development only.
const (
	seedAdminEmail    = "admin@example.com"
	seedAdminName     = "Admin"
	seedAdminPassword = "password1234"
	seedMemberEmail   = "maria@example.com"
	seedMemberName    = "Maria Perez"
)

// seedUsage is the help the seed subcommand prints.
const seedUsage = `Usage:
  alphone seed

Stores the demo data. It takes no flags or arguments.`

// seedCommand runs the seed subcommand, refusing any flag or argument before it touches the database.
func seedCommand(ctx context.Context, getenv func(string) string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	err := flags.Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		_, err := fmt.Fprintln(stdout, seedUsage)
		return err
	case err != nil:
		return fmt.Errorf("seed: %w", err)
	case len(args) > 0:
		return errors.New("seed takes no arguments")
	}
	return seed(ctx, getenv, stdout)
}

// seed migrates the database and stores the demo data set.
func seed(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	stopGrace, err := settingsEnv(getenv).Duration("SHUTDOWN_STOP_GRACE", servingDefaults.StopGrace)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	if err := migrateSchemas(ctx, databaseURL); err != nil {
		return err
	}
	created, err := seedUsers(ctx, pool)
	if err != nil {
		return err
	}
	resolver := contact.NewResolver(postgres.NewContactStore(pool))
	contacts, err := seedContacts(ctx, resolver)
	if err != nil {
		return err
	}
	tasks := postgres.NewTaskStore(pool)
	if err := seedTasks(ctx, tasks, authkitpg.NewUserStore(pool), contacts); err != nil {
		return err
	}
	if err := seedPlugins(ctx, databaseURL, getenv, resolver, stopGrace); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "seeded demo data")
	reportLogins(stdout, created)
	_, _ = fmt.Fprintln(stdout, "development only, never seed a production database")
	return nil
}

// demoLogin is one account the seeder ensures, with the tier it stands in.
type demoLogin struct {
	email string
	name  string
	tier  role.Role
}

// demoLogins names every account the seeder ensures, in banner order.
func demoLogins() []demoLogin {
	return []demoLogin{
		{email: seedAdminEmail, name: seedAdminName, tier: role.Admin},
		{email: seedMemberEmail, name: seedMemberName, tier: role.Member},
	}
}

// seedUsers stores the demo accounts and reports which of them it created.
func seedUsers(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	users := authkitpg.NewUserStore(pool)
	created := map[string]bool{}
	for _, login := range demoLogins() {
		made, err := authkit.EnsureAdmin(
			ctx, users, login.email, login.name, seedAdminPassword, login.tier.String(),
		)
		if err != nil {
			return nil, err
		}
		created[login.email] = made
	}
	if err := seedStatuses(ctx, users, demoStatuses()); err != nil {
		return nil, err
	}
	return created, nil
}

// demoStatus is one account the seeder ensures to show a status other than active.
type demoStatus struct {
	email    string
	name     string
	disabled bool
}

// demoStatuses names the accounts that show the invited and disabled statuses in the users list.
func demoStatuses() []demoStatus {
	return []demoStatus{
		{email: "invited@example.com", name: "Ana Lopez"},
		{email: "disabled@example.com", name: "Luis Garcia", disabled: true},
	}
}

// seedStatuses stores each account as a member awaiting activation, the disabled ones barred, keeping older ones.
func seedStatuses(ctx context.Context, users gouncer.Store, demos []demoStatus) error {
	for _, demo := range demos {
		account, err := gouncer.NewInvitedUser(demo.email, demo.name)
		if err != nil {
			return fmt.Errorf("seed account %s: %w", demo.email, err)
		}
		account.Role = role.Member.String()
		account.Disabled = demo.disabled
		if err := users.CreateUser(ctx, account); err != nil && !errors.Is(err, gouncer.ErrEmailTaken) {
			return fmt.Errorf("seed account %s: %w", demo.email, err)
		}
	}
	return nil
}

// reportLogins names every demo account, saying which ones this run created.
func reportLogins(stdout io.Writer, created map[string]bool) {
	for _, login := range demoLogins() {
		if created[login.email] {
			_, _ = fmt.Fprintln(stdout,
				"login: "+login.email+" / "+seedAdminPassword+" ("+login.tier.String()+")")
			continue
		}
		_, _ = fmt.Fprintln(stdout, login.email+" already exists, its password is unchanged")
	}
}

// demoContact is one contact the seeder ensures, found by its identity.
type demoContact struct {
	channel    contact.Channel
	identifier string
	name       string
}

// Names the synthetic directory contacts are composed from.
var (
	directoryFirstNames = []string{"Alice", "Ben", "Chloe", "Daniel", "Emma", "Felix", "Hannah", "Isaac"}
	directorySurnames   = []string{"Baker", "Carter", "Fisher", "Hughes", "Morgan", "Parker", "Turner", "Walker"}
)

// demoContacts names every contact the seeder ensures, in creation order.
func demoContacts() []demoContact {
	return append([]demoContact{
		{channel: "email", identifier: "ada@example.com", name: "Ada Lovelace"},
		{channel: "email", identifier: "maria.perez@example.com", name: "Maria Perez"},
	}, directoryContacts()...)
}

// directoryContacts returns the synthetic contacts that fill several pages of the contact list.
func directoryContacts() []demoContact {
	contacts := make([]demoContact, 0, len(directoryFirstNames)*len(directorySurnames))
	for _, surname := range directorySurnames {
		for _, firstName := range directoryFirstNames {
			held := demoContact{
				channel:    "email",
				identifier: strings.ToLower(firstName+"."+surname) + "@example.com",
				name:       firstName + " " + surname,
			}
			if len(contacts)%2 == 1 {
				held.channel, held.identifier = "phone", fmt.Sprintf("+1202555%04d", 100+len(contacts))
			}
			contacts = append(contacts, held)
		}
	}
	return contacts
}

// seedContacts stores the demo contacts and returns each one's id by identifier.
func seedContacts(ctx context.Context, resolver *contact.Resolver) (map[string]uuid.UUID, error) {
	demos := demoContacts()
	ids := make(map[string]uuid.UUID, len(demos))
	for _, demo := range demos {
		stored, err := resolver.Resolve(ctx, demo.channel, demo.identifier, demo.name)
		if err != nil {
			return nil, fmt.Errorf("seed contact: %w", err)
		}
		ids[demo.identifier] = stored.ID
	}
	return ids, nil
}

// demoTask is one scripted task of the demo data set.
type demoTask struct {
	id       string
	title    string
	offset   int
	priority int
	done     bool
	contact  string
	origin   string
	member   bool
}

// demoTasks returns the scripted tasks stored by [seedTasks].
func demoTasks() []demoTask {
	return []demoTask{
		{id: "0198d000-0000-7000-8000-000000000001", title: "Chase the overdue invoice", offset: -1},
		{id: "0198d000-0000-7000-8000-000000000002", title: "Call Ada about the renewal", contact: "ada@example.com"},
		{id: "0198d000-0000-7000-8000-000000000003", title: "Approve the new pricing", priority: 1},
		{id: "0198d000-0000-7000-8000-000000000004", title: "File the delivery notes", done: true},
		{
			id:     "0198d000-0000-7000-8000-000000000005",
			title:  "Reply to the imported enquiry",
			offset: 1,
			origin: "seed",
		},
		{
			id:     "0198d000-0000-7000-8000-000000000006",
			title:  "Draft the welcome email",
			member: true,
		},
		{
			id:      "0198d000-0000-7000-8000-000000000007",
			title:   "Follow up with Maria on the offer",
			offset:  3,
			contact: "maria.perez@example.com",
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
	contacts map[string]uuid.UUID,
) error {
	admin, err := users.UserByEmail(ctx, seedAdminEmail)
	if err != nil {
		return fmt.Errorf("seed admin lookup: %w", err)
	}
	colleague, err := users.UserByEmail(ctx, seedMemberEmail)
	if err != nil {
		return fmt.Errorf("seed member lookup: %w", err)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, scripted := range demoTasks() {
		assigneeID := admin.ID
		if scripted.member {
			assigneeID = colleague.ID
		}
		id := uuid.MustParse(scripted.id)
		if _, err := store.Get(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, task.ErrNotFound) {
			return fmt.Errorf("seed task lookup: %w", err)
		}
		built, err := buildDemoTask(scripted, id, today, assigneeID, contacts[scripted.contact])
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
	if scripted.contact != "" {
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

// seedPlugins registers every plugin, starts them and stores their demo data, stopping them within the stop grace.
func seedPlugins(
	ctx context.Context,
	databaseURL string,
	getenv func(string) string,
	resolver *contact.Resolver,
	stopGrace time.Duration,
) error {
	registered, err := registerPlugins(sdk.Deps{
		DatabaseURL: databaseURL,
		Resolver:    resolverBridge{resolver: resolver},
		Contacts:    directoryBridge{resolver: resolver},
		Getenv:      getenv,
	})
	host := pluginkit.NewHost(registered...)
	defer func() { _ = gonsole.StopHost(ctx, host, stopGrace) }()
	if err != nil {
		return err
	}
	wireFieldProviders(registered)
	if err := host.Start(ctx, stopGrace); err != nil {
		return err
	}
	return host.Seed(ctx)
}
