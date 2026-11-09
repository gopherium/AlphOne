// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// lockNotGranted is the code PostgreSQL answers when a row lock cannot be taken in time.
const lockNotGranted = "55P03"

// heldLockTimeout is how long a second transaction waits for a mailbox row another one holds.
const heldLockTimeout = "SET LOCAL lock_timeout = '200ms'"

// mailboxRows counts the mailboxes of one workspace.
func mailboxRows(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID) int {
	t.Helper()
	var rows int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM plugin_email.mailboxes WHERE tenant_id = $1", tenant).Scan(&rows); err != nil {
		t.Fatalf("counting the mailboxes: %v", err)
	}
	return rows
}

// usageOf reads the bytes one workspace keeps, zero when it holds no row.
func usageOf(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID) int64 {
	t.Helper()
	var used int64
	err := pool.QueryRow(t.Context(),
		"SELECT used_bytes FROM plugin_email.workspace_usage WHERE tenant_id = $1", tenant).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatalf("reading the used bytes: %v", err)
	}
	return used
}

// storeMailbox stores the mailbox of one workspace at info@example.com with the alias sales@example.com.
func storeMailbox(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID) mailbox {
	t.Helper()
	box, err := ensureMailbox(t.Context(), pool, tenant, "info@example.com", []string{"sales@example.com"})
	if err != nil {
		t.Fatalf("ensureMailbox() error = %v, want nil", err)
	}
	return box
}

// addBytes adds size to the bytes one workspace keeps in a transaction of its own.
func addBytes(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID, size int64) {
	t.Helper()
	tx := beginTx(t, pool)
	if err := addUsedBytes(t.Context(), tx, tenant, size); err != nil {
		t.Fatalf("addUsedBytes() error = %v, want nil", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("committing the used bytes: %v", err)
	}
}

func TestEnsureMailboxStoresANewMailboxWithItsAliases(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	box := storeMailbox(t, pool, sdk.DefaultTenantID)

	if box.id == uuid.Nil || box.id.Version() != 7 {
		t.Errorf("ensureMailbox() id = %v, want a new UUIDv7", box.id)
	}
	if box.address != "info@example.com" || !slices.Equal(box.aliases, []string{"sales@example.com"}) {
		t.Errorf("ensureMailbox() = %+v, want info@example.com with the alias sales@example.com", box)
	}
	stored, err := readMailbox(t.Context(), pool, sdk.DefaultTenantID, box.id)
	if err != nil || !reflect.DeepEqual(stored, box) {
		t.Errorf("readMailbox() = %+v, %v, want the stored %+v", stored, err, box)
	}
}

func TestEnsureMailboxKeepsTheMailboxAWorkspaceHolds(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	held := storeMailbox(t, pool, sdk.DefaultTenantID)

	box, err := ensureMailbox(t.Context(), pool, sdk.DefaultTenantID, "other@example.com", []string{"ada@example.com"})

	if err != nil {
		t.Fatalf("ensureMailbox() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(box, held) {
		t.Errorf("ensureMailbox() = %+v, want the mailbox the workspace holds, %+v", box, held)
	}
	if rows := mailboxRows(t, pool, sdk.DefaultTenantID); rows != 1 {
		t.Errorf("the workspace holds %d mailboxes, want 1", rows)
	}
}

func TestEnsureMailboxStoresNoAliasAsAnEmptyList(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	box, err := ensureMailbox(t.Context(), pool, sdk.DefaultTenantID, "info@example.com", nil)

	if err != nil {
		t.Fatalf("ensureMailbox() error = %v, want nil", err)
	}
	if box.aliases == nil || len(box.aliases) != 0 {
		t.Errorf("ensureMailbox() aliases = %#v, want an empty list", box.aliases)
	}
}

func TestEnsureMailboxGivesEachWorkspaceItsOwnMailbox(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)

	inDefault := storeMailbox(t, pool, sdk.DefaultTenantID)
	inSecond := storeMailbox(t, pool, second)

	if inDefault.id == inSecond.id {
		t.Errorf("two workspaces share the mailbox %v, want one each", inDefault.id)
	}
	for _, tenant := range []uuid.UUID{sdk.DefaultTenantID, second} {
		if rows := mailboxRows(t, pool, tenant); rows != 1 {
			t.Errorf("workspace %v holds %d mailboxes, want 1", tenant, rows)
		}
	}
}

func TestEnsureMailboxReportsAWorkspaceThatDoesNotExist(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	_, err := ensureMailbox(t.Context(), pool, uuid.Must(uuid.NewV7()), "info@example.com", nil)

	if err == nil || !strings.HasPrefix(err.Error(), "email: ensure mailbox: ") {
		t.Errorf("ensureMailbox() for a missing workspace error = %v, want the store named", err)
	}
}

func TestEnsureMailboxReportsAnIDItCannotMake(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := ensureMailbox(t.Context(), nil, sdk.DefaultTenantID, "info@example.com", nil)

	if !errors.Is(err, errEntropy) {
		t.Errorf("ensureMailbox() error = %v, want %v", err, errEntropy)
	}
}

func TestAWorkspaceHoldsOneMailbox(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	storeMailbox(t, pool, sdk.DefaultTenantID)

	_, err := pool.Exec(t.Context(),
		"INSERT INTO plugin_email.mailboxes (id, tenant_id, address, aliases) VALUES ($1, $2, 'other@example.com', '{}')",
		uuid.Must(uuid.NewV7()), sdk.DefaultTenantID)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		t.Errorf("a second mailbox in one workspace error = %v, want the unique key to refuse it", err)
	}
}

func TestReadMailboxReportsAMailboxThatDoesNotExist(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	_, err := readMailbox(t.Context(), pool, sdk.DefaultTenantID, uuid.Must(uuid.NewV7()))

	if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: read mailbox: ") {
		t.Errorf("readMailbox() error = %v, want no rows named", err)
	}
}

func TestReadMailboxLeavesOutAnotherWorkspacesMailbox(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)
	inSecond := storeMailbox(t, pool, second)

	_, err := readMailbox(t.Context(), pool, sdk.DefaultTenantID, inSecond.id)

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("readMailbox() of another workspace's mailbox error = %v, want no rows", err)
	}
}

func TestReadWorkspaceMailboxAnswersTheMailboxAWorkspaceHolds(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	held := storeMailbox(t, pool, sdk.DefaultTenantID)

	box, found, err := readWorkspaceMailbox(t.Context(), pool, sdk.DefaultTenantID)

	if err != nil || !found || !reflect.DeepEqual(box, held) {
		t.Errorf("readWorkspaceMailbox() = %+v, %t, %v, want %+v found", box, found, err, held)
	}
}

func TestReadWorkspaceMailboxAnswersNoneForAWorkspaceWithNoMailbox(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)
	storeMailbox(t, pool, second)

	box, found, err := readWorkspaceMailbox(t.Context(), pool, sdk.DefaultTenantID)

	if err != nil || found || !reflect.DeepEqual(box, mailbox{}) {
		t.Errorf("readWorkspaceMailbox() = %+v, %t, %v, want none found and no error", box, found, err)
	}
}

func TestReadWorkspaceMailboxReportsAReadThatFails(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	pool.Close()

	_, found, err := readWorkspaceMailbox(t.Context(), pool, sdk.DefaultTenantID)

	if found || err == nil || !strings.HasPrefix(err.Error(), "email: read the workspace mailbox: ") {
		t.Errorf("readWorkspaceMailbox() over a closed pool = %t, %v, want the read named", found, err)
	}
}

func TestLockMailboxBlocksASecondTransaction(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	first, second := beginTx(t, pool), beginTx(t, pool)
	if err := lockMailbox(t.Context(), first, sdk.DefaultTenantID, box.id); err != nil {
		t.Fatalf("lockMailbox() in the first transaction error = %v, want nil", err)
	}
	if _, err := second.Exec(t.Context(), heldLockTimeout); err != nil {
		t.Fatalf("bounding the lock wait of the second transaction: %v", err)
	}

	err := lockMailbox(t.Context(), second, sdk.DefaultTenantID, box.id)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != lockNotGranted {
		t.Errorf("lockMailbox() in the second transaction error = %v, want it held back by the first", err)
	}
	if err := first.Rollback(t.Context()); err != nil {
		t.Fatalf("ending the first transaction: %v", err)
	}
	if err := lockMailbox(t.Context(), beginTx(t, pool), sdk.DefaultTenantID, box.id); err != nil {
		t.Errorf("lockMailbox() once the first transaction ended error = %v, want nil", err)
	}
}

func TestLockMailboxLetsAForeignKeyCheckThrough(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	box := storeMailbox(t, pool, sdk.DefaultTenantID)
	first, second := beginTx(t, pool), beginTx(t, pool)
	if err := lockMailbox(t.Context(), first, sdk.DefaultTenantID, box.id); err != nil {
		t.Fatalf("lockMailbox() error = %v, want nil", err)
	}

	_, err := second.Exec(t.Context(),
		"SELECT 1 FROM plugin_email.mailboxes WHERE tenant_id = $1 AND id = $2 FOR KEY SHARE NOWAIT",
		sdk.DefaultTenantID, box.id)

	if err != nil {
		t.Errorf("the key share lock a foreign key check takes error = %v, want it granted beside the mailbox lock", err)
	}
}

func TestLockMailboxReportsAMailboxThatDoesNotExist(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	err := lockMailbox(t.Context(), beginTx(t, pool), sdk.DefaultTenantID, uuid.Must(uuid.NewV7()))

	if !errors.Is(err, pgx.ErrNoRows) || !strings.HasPrefix(err.Error(), "email: lock mailbox: ") {
		t.Errorf("lockMailbox() error = %v, want no rows named", err)
	}
}

func TestAddUsedBytesCountsEachWorkspaceApart(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	second := storeWorkspace(t, pool)

	addBytes(t, pool, sdk.DefaultTenantID, 100)
	addBytes(t, pool, sdk.DefaultTenantID, 50)
	addBytes(t, pool, second, 7)

	if used := usageOf(t, pool, sdk.DefaultTenantID); used != 150 {
		t.Errorf("the default workspace keeps %d bytes, want 150", used)
	}
	if used := usageOf(t, pool, second); used != 7 {
		t.Errorf("the second workspace keeps %d bytes, want 7", used)
	}
}

func TestAddUsedBytesReportsAWorkspaceThatDoesNotExist(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	err := addUsedBytes(t.Context(), beginTx(t, pool), uuid.Must(uuid.NewV7()), 100)

	if err == nil || !strings.HasPrefix(err.Error(), "email: add used bytes: ") {
		t.Errorf("addUsedBytes() for a missing workspace error = %v, want the counter named", err)
	}
}

func TestOwnAddressesListTheAddressAndEveryAlias(t *testing.T) {
	t.Parallel()

	box := mailbox{address: "info@example.com", aliases: []string{"sales@example.com", "billing@example.com"}}

	got := box.ownAddresses()

	want := []string{"info@example.com", "sales@example.com", "billing@example.com"}
	if !slices.Equal(got, want) {
		t.Errorf("ownAddresses() = %q, want %q", got, want)
	}
}
