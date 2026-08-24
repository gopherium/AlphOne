---
title: Translate AlphOne
description: How to translate the interface into your language, what happens to your work, and when it ships.
---

This page is for you if you speak a language other than English and want
AlphOne to speak it too. You do not need to write code, and you do not
need to use Git.

## What you are translating

AlphOne keeps every sentence the interface shows in a list, separate
from the code. Each entry has an English original and a place for your
translation. The English original is called the source string, and it
never changes when you translate it.

There are four lists rather than one. The core interface keeps one, and
each plugin keeps its own, so the WhatsApp plugin's words live apart
from the core's. Most strings live in the core list, and the plugin
lists are short.

The lists live in a file format called PO, short for Portable Object,
which is the format the GNU gettext tools have used for decades. You
will not have to edit those files by hand. A website does it for you.

## Where the work happens

Translation happens on POEditor, a website built for exactly this. You
sign in, pick your language, and you see the English on one side and a
box for your language on the other. You fill in the boxes. Each of the
four lists is its own project there, and the same account reaches all
of them.

To join, open an issue on the AlphOne repository saying which language
you want to translate. A maintainer invites you to the projects and
switches the language on, since the server keeps its own list of the
languages a reader can pick.

## Things worth knowing before you start

**Placeholders must survive.** Some strings carry a marker like
`%(name)s` or `%(max)d`. These are holes the software fills in with a
name or a number. Copy every marker into your translation exactly as it
appears. You may move a marker to wherever your language needs it, and
you should when word order differs. You may never rename one, drop one
or add one. The repository checks this before your work ships, but a
correct marker saves everyone a round trip.

**Context tells two identical words apart.** English reuses one word for
different things. Status means one thing for an account and another for
a message. When a string carries a context note, translate the meaning
that note describes. Your language may well need two different words
where English used one, and that is the reason the context exists.

**An empty box is safe.** A string you have not translated yet shows in
English. Nothing breaks when a list is half done, so partial work ships
without harm.

**Some words stay in English.** The names AlphOne and WhatsApp are never
translated.

**Write the way the software speaks.** AlphOne addresses the reader
directly and plainly. Keep sentences short, and prefer the everyday word
over the technical one when both exist.

## What happens to your translation

Nothing you translate goes live immediately, and that is deliberate.

Once a week, an automated job collects everything translated since the
last collection and opens a single pull request against the repository.
One request carries every language and every list that moved. If nobody
translated anything that week, no request is opened.

A maintainer reviews and merges it, the same way code is reviewed. Your
work then ships with the next release. A reader picks your language on
their Language screen, and the interface answers in it from then on.

Translations are batched rather than sent one at a time so that
reviewing them stays practical. It also means there is no rush.
Translate at whatever pace suits you, and the next collection will pick
your work up.

## Marking a translation as needing review

A translation can be marked unverified, which gettext calls fuzzy. It
means the words are there but somebody should check them. Use it when
you are unsure, and a later reviewer will see that you flagged it.
