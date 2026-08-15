# Enterprise plugins

This directory is a second plugin root. It works the same way as
`plugins` for every generator, linter and build step, and it is empty
here on purpose.

Closed-source plugins live in a separate private repository. A build
that has that repository checked out beside this one copies them in
here, regenerates the wiring so both roots are seen, and builds from
that throwaway tree. Nothing copied in is ever committed here.

An empty directory changes nothing. Every generator produces the same
bytes with it as without it, so this repository builds, tests and ships
on its own and never needs the private one.

Anything placed here is covered by the LICENSE in this directory rather
than by the Elastic License 2.0 at the repository root.
