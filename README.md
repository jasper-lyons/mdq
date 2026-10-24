# mdq

Query markdown without reading the whole file. The markdown counterpart to
[ctsq](../ctsq).

## The problem

An agent reading a markdown file reads all of it. Across 41k Claude Code turns,
markdown was about a quarter of the token cost of all file reads. The files are
small (median 733 tokens) but they get read again and again: one todo file was
read 163 times across 73 sessions, and 24% of markdown reads were of a file the
session had already read. The usual access pattern is two calls, a grep for
heading lines followed by a `sed` line range guessed from the result.

Markdown is almost never edited after being read, so a read can safely return a
derived view. mdq turns that two call guess into one call that returns exactly
one section or one task list.

## Install

```sh
make install          # builds and installs to ~/.local/bin/mdq
make test             # go test ./...
```

Go 1.25, standard library only, single static binary.

## Commands

```
mdq tree FILE [--depth N]                 outline: headings, line ranges, sizes
mdq toc FILE [--depth N]                  headings and line numbers only
mdq section FILE HEADING|LINE... [flags]  print one or more sections
mdq grep FILE|DIR PATTERN [-i]            search, each hit tagged with its section
mdq tasks FILE|DIR [flags]                list task items
mdq help                                  usage
```

### tree

The cheap first call. One line per heading, indented by level, with the line
range and an approximate token count for the whole section, subsections
included. Token counts are characters over four.

```sh
mdq tree docs/todo.md --depth 2
docs/todo.md  4338 lines  ~81.8k tokens  93 headings
   1-4338  ~81.8k  # Mavexa TODO / Follow-ups
   11-166   ~3.2k    ## Deploy gate CLEAR (opened 2026-08-04, closed 2026-08-05)
  276-346   ~1.1k    ## LLM
  348-453   ~1.8k    ## Chat / conversation
  455-876   ~7.4k    ## Contacts / CRM (iteration-2 N1/N2)
 878-1206   ~5.4k    ## Delivery / public pages
```

Content that sits after the front matter and before the first heading is listed
as `(preamble)` so it is never invisible.

### toc

The same outline with no sizes and no indentation, one `file:line: heading` per
line, for piping.

```sh
mdq toc docs/todo.md
docs/todo.md:1: # Mavexa TODO / Follow-ups
docs/todo.md:11: ## Deploy gate CLEAR (opened 2026-08-04, closed 2026-08-05)
docs/todo.md:124: ### Fix batches merged without being applied
```

### section

Prints one section. The heading line comes back prefixed `FILE:LINE:`, then the
body verbatim. Subsections are included by default.

```sh
mdq section docs/guide.md 'Package: api'
docs/guide.md:42: ## Package: api

Owns the HTTP surface. Handlers live in internal/api.
...
```

```
--no-children      stop at the first subheading
--no-body          print the subsection outline instead of the prose
--max-lines N      truncate the body, default 200
```

`--no-body` is the middle step between `tree` and a full read. It answers "what
is in here" for the price of a few lines.

```sh
mdq section docs/todo.md 'LLM' --no-body
docs/todo.md:276: ## LLM
278-298  ~327    ### Structured per-call telemetry (cost-per-accepted-output)
300-319  ~316    ### Brand voice across all user-facing roles
321-346  ~446    ### Model-chosen page structure (dynamic skeleton planning)
(section 276-346, 71 lines, ~1.1k tokens, body not printed)
```

A truncated body ends with a marker rather than stopping silently:

```
... (147 more lines, use --max-lines or a narrower heading)
```

Several headings in one call print several sections, in the order given. Each
one keeps its own `FILE:LINE:` header, so the output says where every block came
from, and `--max-lines` applies per section rather than to the whole run.

```sh
mdq section docs/guide.md 'Conventions' 'Endpoints' 'Package: backend'
docs/guide.md:6: ## Conventions
...

docs/guide.md:157: ## Endpoints
...

docs/guide.md:298: ## Package: backend
...
```

Quote each heading. An unquoted multi word heading arrives as several arguments,
and mdq only joins them back up when they fail to resolve one by one and the
joined form resolves to a single heading.

A heading that does not resolve reports itself and the remaining headings still
print. A missing one names the file it searched and offers the nearest headings
it did find:

```sh
mdq section docs/todo.md 'Nonexistent heading'
no heading matching 'Nonexistent heading' in docs/todo.md (93 headings)
closest headings: "Setext heading rules" :212
try a shorter heading or `mdq toc docs/todo.md`
```

An ambiguous one lists its candidates as before. The exit code is 0 when every
heading resolved, 1 when none did, and 2 when some resolved and others did not,
or when any was ambiguous.

### grep

Search, but every hit says which section it sits in, so there is no follow-up
read to find out where you landed. The pattern is a Go regular expression and
the search is case sensitive unless you pass `-i`. A directory argument is
walked for `.md`, `.markdown` and `.mdx` files, sorted.

Alternation is `A|B`. In Go a `\|` is a literal pipe rather than alternation, so
`mdq grep FILE 'A\|B'` written out of GNU grep habit would find nothing. mdq
rewrites `\|` to `|` when the rest of the pattern is plain text. A pattern
carrying any other regex syntax is left as written, and if such a pattern finds
nothing the no match message adds a note saying the `\|` was a literal pipe.

```sh
mdq grep docs/todo.md 'unsubscribe'
docs/todo.md  2 hits
1044 [Delivery / public pages > No unsubscribe or suppression an...]: ### No unsubscribe or suppression anywhere CLOSED (built 2026-08-15)
1046 [Delivery / public pages > No unsubscribe or suppression an...]: Built: (workspace, email) suppression + durable unsubscribe tokens gate invite and follow-up waves.
```

The path is a header line, once per file, then one line per hit. The bracket
shows the two innermost enclosing headings.

No match is still exit 1, but it prints an answer rather than nothing. The first
line says what was searched and how big it was, then up to three headings that
share words with the pattern, then the next thing to try:

```sh
mdq grep docs/todo.md 'Asset chat image swap'
no match for /Asset chat image swap/ in docs/todo.md (4338 lines, 93 headings)
closest headings: "Asset chat - session-only by design (slice 6)" :410
                  "Item library - the deliberate gaps" :1398
try a shorter pattern or `mdq toc docs/todo.md`
```

A heading is only offered when it shares at least one word with the pattern, so
a pattern with nothing like it in the document prints the first line alone. A
directory target reports the files scanned instead of the lines, and each
suggested heading carries its file path.

### tasks

Lists task items. Open items only by default.

```sh
mdq tasks ~/Personal/Pages/Projects.md --section Doing
/Users/jasperlyons/workspace/Personal/Pages/Projects.md  20 open, 0 done
 87 [Doing] Mavexa: sales kit design decided and folded into stories, sub-issues ENG-514 to 520...
 88 [Doing] Mavexa v2.0 product overhaul (vision, stories, MVP, Linear, implementation)
 89 [Doing] Act on ROI doc comments: cost fields to build, rename attribution to influence #mavexa
```

```
--open             open items only, the default
--done             completed items only
--all              both
--tag T            only items carrying #T
--section H        only items under the heading H
--no-bullets       drop bold lead bullets, keeping checkbox and heading items
--json             machine readable output
--width N          truncate item text, default 160, 0 for no limit
--max-lines N      cap the number of items printed, default 200
```

The path is a header line, once per file, with the counts behind the filters
you asked for. Then one item per line, `LINE [Section] text`. Completed items
carry `[x]` before the text. A nested item keeps its parent for context after a
`<`:

```sh
mdq tasks ~/Personal/Pages/Projects.md --all --tag myrge
/Users/jasperlyons/workspace/Personal/Pages/Projects.md  0 open, 5 done
506 [Archive] [x] Jasper bp Onboarding #myrge
508 [Archive] [x] Review Offer in Candidate Zone  < Jasper bp Onboarding #myrge
```

A long list stops at `--max-lines` and says how to shorten it:

```
... (84 more items, narrow with --section or --tag, or raise --max-lines)
```

Hashtags stay inline in the text rather than being repeated at the end of the
line. `--json` returns file, line, done, text, tags, section, path, parent and
kind for every item, carries the full path on each one, and never truncates.

## Heading matching

`mdq section` and `mdq tasks --section` take the same kind of argument.

- Case insensitive.
- Leading `#` characters, leading emoji and markup characters are ignored, so
  `'Deploy gate'` finds `## ✅ Deploy gate — CLEAR (opened 2026-08-04)`.
- Trailing status text is ignored for matching. A trailing parenthetical, or a
  trailing segment after a dash or colon that starts with a status word, is not
  part of the name.
- A unique prefix works. So does a unique substring.
- `/regex/` matches the heading text case insensitively.
- A bare number is a line number. It resolves to the heading on that line, or
  to the innermost section containing it. To address a numbered heading such
  as `### 07. Queue writes`, include the hashes or a word from the text
  (`'### 07'` or `'Queue writes'`), because a bare `07` is read as a line.

Matching runs in tiers: exact names first, then prefixes, then substrings. The
first tier with any match wins. Several matches in that tier print the
candidates with line numbers and exit 2, so the caller can pick one.

`mdq section` takes any number of headings and resolves each on its own, so
`mdq section guide.md 'Conventions' 'Endpoints'` prints two sections. Quote each
one. Bare numbers are still line numbers, one per argument.

## Task grammar

Settled after inspecting an Obsidian kanban board and a 4,338 line prose
follow-ups file. Two shapes count as tasks, and a third is available behind a
flag.

**1. Checkbox items.** A list item whose marker is followed by a bracket state,
at any indent, under any list marker (`-`, `*`, `+`, `1.`, `1)`).

```
- [ ] open
- [x] done
- [/] open, in progress in Obsidian
- [-] done, cancelled in Obsidian
	- [ ] nested, keeps its parent as context
```

`[ ]`, `[/]`, `[<]`, `[>]`, `[!]`, `[?]` are open. `[x]`, `[X]`, `[-]`, `[~]`
are done. Indentation is measured with a tab as four columns. An item indented
further than the one above it is a child of it, and prints with ` < parent`.

**2. Headings that carry a status marker.** A heading at level 2 or deeper is a
task when it carries one of these, and is not a task otherwise. Level 1 is the
document title in every file shape seen so far, so it is never a task.

| marker | example | state |
| --- | --- | --- |
| leading done glyph | `## ✅ Deploy gate` | done |
| leading open glyph | `## 🚧 Half built` | open |
| leading `~~` | `## ~~Pull from Salesforce~~` | done |
| an upper case status word anywhere | `## Fix the thing DONE` | done |
| an upper case open word anywhere | `## TODO write the migration` | open |
| a trailing segment after a dash or colon that starts with a status word | `## P4 shipped before P3 — resolved: ...` | done |

Done words are DONE, CLOSED, CLEAR, FIXED, RESOLVED, WONTFIX, SUPERSEDED,
CANCELLED, SHIPPED. Open words are TODO, WIP, BLOCKED, IN PROGRESS, PENDING.
Case matters for the "anywhere" rule, because a lower case "fixed" in prose is
not a status. Case does not matter in a trailing status segment, because the
position already carries the meaning.

**3. Bold lead bullets.** In a prose follow-ups file the real items are often
`- **Title.** body` bullets, hundreds of them, and they count by default. A
task query that silently drops hundreds of items is wrong more often than one
that is long, and a long one says how to narrow itself. `--no-bullets` leaves
only the checkbox and heading items. `--bullets` is accepted and does nothing.

A bullet takes its state from the same status rules applied to its first line,
and it is also done when a bold status marker such as `**FIXED**` or
`**CLOSED**` appears later in the same item, which is how a prose follow-up
records its own outcome. The item ends at the next bullet at the same indent,
the next heading, or a blank gap.

A task's section is the innermost enclosing heading. For a heading task it is
the parent heading.

## Parsing notes

- YAML front matter is skipped when the file opens with `---` and a closing
  `---` or `...` follows within the first 200 lines.
- Fenced code blocks are tracked, so a `#` or a `- [ ]` inside a fence is not a
  heading or a task. Both backtick and tilde fences are handled.
- Setext headings are recognised. Per CommonMark, a line of dashes directly
  under a paragraph line is a level 2 heading rather than a thematic break,
  which is how GitHub and Obsidian render it too. In a document that uses `---`
  as a separator this shows up as an extra heading whenever the author left out
  the blank line above it.
- A section ends at the next heading of the same level or shallower, and its
  reported range is trimmed back to the last non blank line.

## Exit codes

```
0   success
1   no match
2   ambiguous match, a partly resolved section list, or a usage error
```

Output is deterministic. The same input always produces byte identical output,
and mdq never writes to the files it reads.
