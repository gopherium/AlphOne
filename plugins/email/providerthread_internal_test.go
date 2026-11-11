// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mailDay is one day of received time.
const mailDay = 24 * time.Hour

// conversationInvoice returns a plain invoice from Maria Perez with no reply headers, its Message-ID naming day.
func conversationInvoice(day int) []byte {
	return threadedMail("From: maria.perez@example.com", "To: info@example.com", "Subject: Invoice",
		fmt.Sprintf("Message-ID: <invoice-day-%d@example.com>", day))
}

// conversationDay returns the arrival on day under the provider thread id file:conversation.
func conversationDay(day int) arrival {
	return arrival{providerThread: "file:conversation", after: time.Duration(day) * mailDay}
}

// everyOrder returns every order of days.
func everyOrder(days []int) [][]int {
	if len(days) <= 1 {
		return [][]int{slices.Clone(days)}
	}
	var orders [][]int
	for i, first := range days {
		for _, rest := range everyOrder(slices.Concat(days[:i], days[i+1:])) {
			orders = append(orders, append([]int{first}, rest...))
		}
	}
	return orders
}

// daysByThread stores the invoices of the days in order inside a savepoint it rolls back, and lists each thread's days.
func (tt *threadTest) daysByThread(order []int) string {
	tt.t.Helper()
	sub, err := tt.tx.Begin(tt.t.Context())
	if err != nil {
		tt.t.Fatalf("beginning a savepoint: %v", err)
	}
	defer func() { _ = sub.Rollback(tt.t.Context()) }()
	fed := &threadTest{t: tt.t, tx: sub, box: tt.box, contacts: tt.contacts}
	mails := map[int]uuid.UUID{}
	for _, day := range order {
		_, mails[day] = fed.store(fed.mail(conversationInvoice(day), conversationDay(day)))
	}
	days := map[uuid.UUID][]int{}
	for day, mail := range mails {
		thread := storedRow(tt.t, sub, mail).threadID
		days[thread] = append(days[thread], day)
	}
	if threads := fed.threads(); threads != len(days) {
		tt.t.Errorf("the order %v leaves %d threads, %d of them holding mail", order, threads, len(days))
	}
	groups := make([]string, 0, len(days))
	for _, held := range days {
		slices.Sort(held)
		groups = append(groups, fmt.Sprint(held))
	}
	slices.Sort(groups)
	return strings.Join(groups, " ")
}

func TestAMailUnderAProviderThreadIDJoinsTheThreadNearItsOwnTime(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	first := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice",
		"Message-ID: <invoice-march@example.com>"), arrival{providerThread: "file:conversation"})
	second := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice",
		"Message-ID: <invoice-april@example.com>"), arrival{providerThread: "file:conversation", after: 30 * mailDay})
	if second == first {
		t.Fatal("the second invoice joined the first thread, want a new one")
	}

	third := tt.reach(threadedMail("From: maria.perez@example.com", "Subject: Invoice question",
		"Message-ID: <invoice-question@example.com>"), arrival{providerThread: "file:conversation", after: 31 * mailDay})

	if third != second {
		t.Errorf("a mail a day after the second invoice under its provider thread id sits in %v, want %v (first %v)",
			third, second, first)
	}
	if threads := tt.threads(); threads != 2 {
		t.Errorf("the mailbox holds %d threads, want 2", threads)
	}
}

func TestAnOlderMailFedAfterANewerProviderThreadJoinsTheThreadOfItsOwnTime(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	march := tt.reach(conversationInvoice(0), conversationDay(0))
	april := tt.reach(conversationInvoice(30), conversationDay(30))

	got := tt.reach(conversationInvoice(1), conversationDay(1))

	if got != march {
		t.Errorf("a mail of day 1 fed after day 30 sits in %v, want the day 0 thread %v (day 30 %v)", got, march, april)
	}
	if threads := tt.threads(); threads != 2 {
		t.Errorf("the mailbox holds %d threads, want 2", threads)
	}
}

func TestMailOfOneContactUnderOneProviderThreadIDGivesTheSameThreadsInEveryOrder(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	for _, fed := range []struct {
		days []int
		want string
	}{
		{days: []int{0, 1, 30, 31}, want: "[0 1] [30 31]"},
		{days: []int{0, 10, 20, 40, 41}, want: "[0 10 20] [40 41]"},
	} {
		orders := everyOrder(fed.days)
		count := len(orders)

		wrong := slices.DeleteFunc(orders, func(order []int) bool { return tt.daysByThread(order) == fed.want })

		if len(wrong) > 0 {
			t.Errorf("%d of %d orders of the days %v give other threads than %s, the order %v gives %s",
				len(wrong), count, fed.days, fed.want, wrong[0], tt.daysByThread(wrong[0]))
		}
	}
}

func TestALateMailWithinTheWindowOfTwoThreadsOfItsProviderThreadIDJoinsThem(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	first := tt.reach(conversationInvoice(0), conversationDay(0))
	tt.reach(conversationInvoice(20), conversationDay(20))

	got := tt.reach(conversationInvoice(10), conversationDay(10))

	if threads := tt.threads(); got != first || threads != 1 {
		t.Errorf("a mail of day 10 between the threads of days 0 and 20 joined %v with %d threads, want %v alone",
			got, threads, first)
	}
	keys := countRows(t, tt.tx, "SELECT count(*) FROM plugin_email.thread_keys WHERE kind = $1", providerThreadKind)
	if keys != 1 {
		t.Errorf("the mailbox holds %d rows of the provider thread id, want 1", keys)
	}
}

func TestALateMailNearThreadsOfTwoContactsJoinsTheNearestAndMergesNothing(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	ada := tt.reach(threadedMail("From: ada@example.com", "To: info@example.com", "Subject: Invoice",
		"Message-ID: <invoice-ada@example.com>"), conversationDay(0))
	maria := tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com", "Subject: Invoice",
		"Message-ID: <invoice-maria@example.com>"), conversationDay(10))
	sent := conversationDay(7)
	sent.sentCopy = true

	got := tt.reach(threadedMail("From: info@example.com", "To: maria.perez@example.com, ada@example.com",
		"Subject: Invoice", "Message-ID: <invoice-both@example.com>"), sent)

	if threads := tt.threads(); got != maria || threads != 2 {
		t.Errorf("a Sent copy of day 7 to both contacts joined maria %t, ada %t, with %d threads, want maria's and 2",
			got == maria, got == ada, threads)
	}
}

func TestAMailBetweenTheFirstAndLastMailOfAProviderThreadJoinsIt(t *testing.T) {
	t.Parallel()

	tt := newThreadTest(t)
	var thread uuid.UUID
	for _, day := range []int{0, 12, 24, 36} {
		thread = tt.reach(conversationInvoice(day), conversationDay(day))
	}

	got := tt.reach(conversationInvoice(18), conversationDay(18))

	if threads := tt.threads(); got != thread || threads != 1 {
		t.Errorf("a mail of day 18 in a thread from day 0 to day 36 joined %v with %d threads, want %v alone",
			got, threads, thread)
	}
}

func TestAMailOutsideEverySpanOfItsProviderThreadIDJoinsOnlyByItsOwnHeaders(t *testing.T) {
	t.Parallel()

	for _, late := range []struct {
		name    string
		headers []string
		joins   string
	}{
		{name: "no reply id"},
		{name: "a reply id no stored mail holds", headers: []string{"In-Reply-To: <unknown@example.com>"}},
		{name: "the reply id of the day 0 invoice", headers: []string{"In-Reply-To: <invoice-day-0@example.com>"},
			joins: "day 0"},
		{name: "a Thread-Index the day 30 thread holds", headers: []string{"Thread-Index: " + threadIndex(0x44)},
			joins: "day 30"},
	} {
		t.Run(late.name, func(t *testing.T) {
			t.Parallel()

			tt := newThreadTest(t)
			threads := map[string]uuid.UUID{
				"day 0":  tt.reach(conversationInvoice(0), conversationDay(0)),
				"day 30": tt.reach(conversationInvoice(30), conversationDay(30)),
			}
			tt.reach(threadedMail("From: maria.perez@example.com", "To: info@example.com", "Subject: Invoice",
				"Message-ID: <invoice-day-31@example.com>", "Thread-Index: "+threadIndex(0x44)), conversationDay(31))

			got := tt.reach(threadedMail(slices.Concat([]string{"From: maria.perez@example.com", "To: info@example.com",
				"Subject: Invoice", "Message-ID: <late@example.com>"}, late.headers)...), conversationDay(50))

			want, joins := threads[late.joins]
			if joins && got != want {
				t.Errorf("a mail of day 50 with %s joined %v, want the %s thread %v", late.name, got, late.joins, want)
			}
			if !joins && slices.Contains(slices.Collect(maps.Values(threads)), got) {
				t.Errorf("a mail of day 50 with %s joined the stored thread %v, want a new thread", late.name, got)
			}
		})
	}
}
