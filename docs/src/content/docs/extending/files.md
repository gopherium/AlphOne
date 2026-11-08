---
title: Files
description: Keep the files your plugin needs, such as the attachments of a mail, apart for each workspace, and serve them as safe downloads.
---

A plugin that needs to keep files, such as the attachments of a mail,
gets a file store from AlphOne. It saves a file under a new id or under
one it chose, opens it again by its id and deletes it. The store keeps
each file under the workspace and the plugin that saved it, so no other
workspace and no other plugin ever reaches it.

Where the files live is the operator's choice, in PostgreSQL or in an S3
bucket, see [Files](/self-hosting/configuration/#files). Your plugin
works the same either way.

## Take a store

Implement `sdk.FileStoreConsumer`. AlphOne calls its one method after
every plugin has registered and before any plugin starts:

```go
var _ sdk.FileStoreConsumer = (*Plugin)(nil)

// UseFileStore keeps the file store AlphOne hands the plugin.
func (p *Plugin) UseFileStore(store sdk.FileStore) {
	p.files = store
}
```

Every plugin that implements it gets a store of its own. The `var _`
line turns a misspelled method into a build error, so the plugin never
runs without its store.

## Save a file

`Save` reads the body to its end, or until it passes [the cap](#the-cap),
and stores a body within the cap under a new id:

```go
file, err := p.files.Save(ctx, "application/pdf", body)
if errors.Is(err, sdk.ErrFileTooLarge) {
	return errAttachmentTooLarge
}
if err != nil {
	return err
}
```

The answer is a `sdk.StoredFile`:

| Field | What it holds |
| --- | --- |
| `ID` | The id the file is kept under. |
| `Size` | The length of the file in bytes. |
| `ContentType` | The media type you passed in its standard form, such as `text/plain` for `Text/Plain`, or `application/octet-stream` when you passed none. |
| `SHA256` | The SHA-256 of the bytes in lower case hex. |
| `StoredAt` | When AlphOne stored the file. |

AlphOne keeps no list of your files. Keep what `Save` answers in a table
of your own, beside what the file belongs to, such as the message it
came with. Keep the id, and the type, size, SHA-256 and time you need to
serve the file. `Open` answers only the bytes, and without the id nobody
finds the file again.

AlphOne never guesses a media type from the bytes. A type that does not
read as one, such as `image/png; =`, or that names no subtype, such as
`inline`, is refused before anything is stored.

## Choose the id yourself

`Save` picks the id, so you learn it only once the file is stored. If
your plugin stops between the save and the write of its row, nothing
points at the file and nobody can find it to delete it. `SaveAs` takes
an id you choose, so the row comes first and the file second:

```go
id := uuid.Must(uuid.NewV7())
err := p.attachments.Add(ctx, messageID, id)
if err != nil {
	return err
}
file, err := p.files.SaveAs(ctx, id, "application/pdf", body)
if err != nil {
	return err
}
return p.attachments.MarkStored(ctx, file)
```

A row that was never marked stored names the id to clean up after a
crash. Call `Delete` on that id, which answers `sdk.ErrFileNotFound`
when no file was stored, then delete the row.

This cleanup is safe only when the id is unique in your table, as its
primary key or a unique column. Then `Add` fails for a second row with
the same id, so the file under the id of a row never marked stored is
always that row's own. If two rows could hold one id, `SaveAs` would
answer `sdk.ErrFileExists` for the second, and the cleanup would delete
the file of the first.

The cleanup also needs an age. Take only the rows added longer ago than
a save can take, so give each row the time it was added. A newer row may
belong to a save that is still running. The cleanup would delete that
row, the save would then store its file, and nothing would point at the
file. Read that age from your plugin's [settings](/extending/settings/),
never from a number written in your code.

Every rule of `Save` holds for `SaveAs`: [the cap](#the-cap), the media
type check and [the workspace](#every-call-serves-one-workspace) and
plugin of the call. An id that already holds a file of the same
workspace and plugin answers `sdk.ErrFileExists`, and the file it holds
stays as it was. So make a new id for every file, such as a version 7
UUID, and never take one from the content, because two files with the
same bytes would then share it.

AlphOne looks the id up before it saves and asks the store to refuse a
second file under it. Some S3 compatible stores, Garage v2.4.1 among
them, ignore that request, so there two saves that run at the same
moment under one id can still replace each other. A new id for every
file never meets that case.

## The cap

Each file holds at most `ALPHONE_STORAGE_MAX_BYTES` bytes, 25 MiB unless
the operator sets another cap. A longer body answers
`sdk.ErrFileTooLarge` and nothing is stored. `Save` stops reading such a
body one byte past the cap. A body whose read fails
answers that failure, and nothing is stored either.

`Save` streams the body to where the files live as it reads it, so a
large file never sits whole in memory.

## Open and delete a file

`Open` answers the file under an id as an `io.ReadSeekCloser`. Close it
when you are done:

```go
content, err := p.files.Open(ctx, row.FileID)
if errors.Is(err, sdk.ErrFileNotFound) {
	w.WriteHeader(http.StatusNotFound)
	return
}
if err != nil {
	w.WriteHeader(http.StatusInternalServerError)
	return
}
defer content.Close()
```

`Delete` removes the file under an id. Delete the file when you delete
the row that holds its id, so no file is left that nothing points at.
`Open` and `Delete` answer `sdk.ErrFileNotFound` for an id the store
holds no file under.

## Every call serves one workspace

Each call works in the workspace `ctx` carries. AlphOne puts it on `ctx`
for every signed in request. Work your plugin starts on its own, such as
a background sweep, sets it with `sdk.WithTenant`. A `ctx` that carries
no workspace works in the default one.

An id saved in one workspace answers `sdk.ErrFileNotFound` in every
other workspace, and in every other plugin, on `Open` and on `Delete`. A
plugin cannot reach another workspace's files even when it knows the id.

## Serve a file as a download

`sdk.ServeDownload` sends a stored file to a browser. Check first that
the caller may see the file, for example by finding its id in your own
table beside the conversation the caller opened. The details come from
that row:

```go
sdk.ServeDownload(w, r, sdk.Download{
	Content:     content,
	ContentType: row.ContentType,
	Filename:    row.Filename,
	SHA256:      row.SHA256,
	ModTime:     row.StoredAt,
})
```

It answers range requests, so a browser can jump ahead in a video or
resume a download. It sends the SHA-256 you pass as the `ETag`. It also
protects the people who open the file:

- `X-Content-Type-Options: nosniff` stops the browser from reading the
  file as any type other than the one sent.
- `Content-Security-Policy: sandbox` runs nothing the file holds, so a
  page or a script someone sent cannot act as your site.
- Only photos in JPEG, PNG or WebP, audio, and videos in MP4 or 3GPP
  show inline. Every other file downloads as an attachment, under
  `Filename` when you name one, so an HTML or SVG file never opens as a
  page.
- `Cache-Control: private, no-store` keeps every cache, the browser's
  own included, from keeping the file. Each new view asks AlphOne again,
  so a person who signed out or lost access cannot open it from the
  cache.

`ServeDownload` reads any `io.ReadSeeker`. The WhatsApp plugin serves the
media it keeps in its own table through it too.
