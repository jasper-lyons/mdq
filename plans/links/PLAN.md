# mdq links

## Goal

`mdq links FILE|DIR` lists the links out of markdown files, each tagged with
the section it sits in, so a separate browser (ctsq's explore) can follow
links between files. mdq stays a reader: it never follows, fetches or opens a
link, it only reports them.

Precedence: where this plan is silent, the existing code's conventions are
the contract (internal/md/tasks.go and internal/cli/tasks.go are the model).

## Decisions

### 01. Placement mirrors tasks

Extraction lives in a new `internal/md/links.go`:

```go
type Link struct {
    File        string
    Line        int
    Text, Target, Path, Anchor, Kind string
    Section     string
    SectionPath []string
}
func (d *Document) Links() []Link
```

modelled on `Tasks` in `internal/md/tasks.go`. JSON tags follow the style of
the `Task` struct. The command is `cmdLinks` in a new `internal/cli/links.go`,
plus one `case "links"` in the switch at `internal/cli/cli.go:65-98` and one
usage line in the usage const at `cli.go:24-62`. Flags are parsed with
`parseArgs`/`flagSet` (`internal/cli/args.go:18,82`).
Why: one pattern for every subcommand; beat a separate links package.

### 02. Link kinds

`Kind` is one of these strings:

- `inline`: `[text](target)`, including a link whose text or target is split
  across a line break. It is reported at the line where `[` starts.
- `autolink`: `<http://...>`, `<https://...>`, `<mailto:...>`.
- `wiki`: `[[x]]` (Text and Target both `x`; `[[x|label]]` gives Target x,
  Text label).
- `code`: an inline code span whose whole content is a path ending in `.md`,
  `.markdown` or `.mdx` (optionally followed by `#anchor`), such as
  `` `docs/x.md` ``. ON by default: it is the dominant form in the target
  corpus (2,473 in Mavexa against about 290 inline links). A code span with
  spaces, or any other content, is not a link.
- `bare`: a bare `http://` or `https://` URL in prose, only with `--bare`.
  Trailing sentence punctuation (`.,;:!?)` and a closing `)` with no opener)
  is not part of the URL.

Out of scope: reference-style links (0 in the corpus) and prose references
such as "Spec 03" or "Decision 07". Image links `![alt](x)` are not links;
skip them.

### 03. What is skipped

Fenced code blocks (`doc.Fenced`) and front matter are skipped entirely.
Inline code spans are stripped before matching the other kinds, so
`` `[a](b.md)` `` is not an inline link; the span itself is only ever a
`code` candidate.

### 04. Internal versus external

A link is external when its target starts with `http://`, `https://` or
`mailto:`; every other target (a relative path, or `#anchor`) is internal.
`--internal` keeps only internal links, `--external` only external ones; both
together is a usage error (exit 2).

`Anchor` is split off the target at the first `#`:
`glossary.md#effective-kind` gives Target `glossary.md`, Anchor
`effective-kind`; `#x` gives an empty Target and Anchor `x`. External links
keep their full URL in Target and leave Anchor empty.

### 05. Path resolution

For internal links only, always on, no flag:

- Empty Target (a same-file anchor): Path is the file itself.
- Otherwise resolve Target against the directory of the file.
- For a `code` link whose resolved path does not exist, try the Target
  against each ancestor directory in turn, stopping after the repo root
  (the first directory, walking up, that contains `.git`; with no `.git`
  found, stop at the filesystem root). The first that exists wins.
- Path is printed relative to the current working directory (absolute if
  that fails). When no candidate exists, Path is the first candidate (the
  one next to the file) and the link is marked `(missing)`. In JSON this is a
  `Missing bool` field on Link; in text, ` (missing)` after the path.

External links have an empty Path.

### 06. Output

Text output matches `mdq grep` (`internal/cli/cli.go:353-361`): per file a
header `<path>  N links`, then one row per link:

```
%*d [Section > Sub]: <target>  -> <path>
```

where the tag comes from `where(doc, line)` (`cli.go:429`), `<target>` is the
original target as written (anchor included), and `  -> <path>` is present
for internal links only, followed by ` (missing)` when missing. Files with no
links print nothing.

Flags: `--section H` (restrict to one section, as in tasks), `--width`,
`--max-lines` (as in grep), `--json` (an indented array of Link like tasks
`--json` at `internal/cli/tasks.go:93-104`; `[]` when empty), `--internal`,
`--external`, `--bare`.

Exit codes follow `cli.go:18-22`: 0 when at least one link is printed, 1 when
none (including `--json` printing `[]`), 2 on a usage error. A DIR argument
is walked with `collect(path)` (`cli.go:445`).

## Conventions

- Go standard library only. No new dependencies.
- Match the surrounding code: naming, comment density, error style, test
  style (in-process `run(...)` for CLI tests, `requireFile` for real-file
  tests).
- Gates, all must pass: `go vet ./...`, `go test ./...`, `go build -o mdq .`.
- Never run `make install`: it replaces the mdq binary every session uses.
- Do not edit README.md (another change to it is in flight elsewhere).

## Package: links

Tier: Sonnet. Single package, no dependencies.

Files owned:

- `internal/md/links.go` (new)
- `internal/md/links_test.go` (new)
- `testdata/links.md` (new), plus any small fixture tree under `testdata/`
  needed for ancestor resolution and `(missing)` (e.g.
  `testdata/linkrepo/...`)
- `internal/cli/links.go` (new)
- `internal/cli/cli.go`: additive only, one `case "links"` and one usage line
- `internal/cli/cli_test.go`: additive only, new test functions

Acceptance:

1. Unit tests over `testdata/links.md` cover every kind, a link split across
   lines (reported at its first line), fenced code skipped, front matter
   skipped, an inline code span that is not a path ignored, an inline link
   inside a code span ignored, anchors (`x.md#a` and `#a`), `(missing)`, and
   ancestor resolution for a code-span path.
2. CLI tests cover `--json` (shape and `[]` on empty), `--internal`,
   `--external`, both together exits 2, `--section`, `--bare` (off by
   default, on with the flag), exit 0 with links, exit 1 with none, and a
   DIR argument.
3. One real-file test guarded by `requireFile`:
   `~/workspace/projects/mavexa/mavexa/docs/pages-and-pipeline.md` yields a
   link with Target `glossary.md`, Anchor `effective-kind` at line 234.
4. All three gates pass.
