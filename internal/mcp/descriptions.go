// SPDX-License-Identifier: Elastic-2.0

package mcp

// workloadDescription tells an agent what workload_summary answers.
const workloadDescription = `Report how much work the signed in user holds right now.

Answers two numbers, how many open tasks are due today and how many were due
earlier and are still open. Use this first when someone asks about their day,
their workload, or what is outstanding. It takes no arguments and reads only
the tasks of the user whose token opened this session.

Counts are read one page deep. When more tasks exist past a page the answer
sets capped to true and that count is a lower bound, so say "at least" when
reporting it.`

// tasksDescription tells an agent what list_my_tasks answers.
const tasksDescription = `List the signed in user's tasks for one day, or the overdue backlog.

Reads only the tasks of the user whose token opened this session. Give date to
list one day, or due_before to list everything still open from before that day.
Naming both is refused. Naming neither lists today.

Dates are calendar days written YYYY-MM-DD, for example 2026-08-14. Status is
open, done, or all, and defaults to open. Priority runs 0 to 9 where higher
is more urgent and 0 is the normal default. Each task carries its contact's
name beside the contact id when it is linked to one, so there is no need to
look the contact up separately. Every task here belongs to the caller, so
assignee_id is always the caller's own user id, which is how to tell the
caller's work apart in contact answers.`

// contactsDescription tells an agent what find_contacts answers.
const contactsDescription = `Search the contact directory and report who holds open work.

Give a query to match a contact name or a channel address. Digits are pulled
out of the query, so "184 467" finds the identity 184467235. A blank query
lists contacts up to the limit.

Each answer says whether that contact holds at least one open task. That flag
counts every user's tasks, not only the caller's, the same way the contact
page in AlphOne shows the contact's work across the whole account. To read
one contact in full, pass the id from here into get_contact.`

// contactDescription tells an agent what get_contact answers.
const contactDescription = `Read one contact in full, with its channel addresses and open tasks.

Takes the contact uuid. Take that id from find_contacts or from a task's
contact_id. Never guess a uuid and never pass a name here, an id AlphOne does
not hold is refused.

Open tasks span every user, not only the caller, and each carries the
assignee_id of the user it belongs to. Compare it against the caller's own
assignee_id from list_my_tasks to tell whose work is whose. Priority runs
0 to 9 where higher is more urgent and 0 is the normal default. When
open_tasks_capped is true the contact holds more open tasks than this answer
carries.`
