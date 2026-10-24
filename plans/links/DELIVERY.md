# mdq links — delivery

## What's been built

`mdq links FILE|DIR` is a new command. It lists every link in one markdown file, or in every markdown file under a directory. Each link is shown with its line number and the section heading it sits under. For a link to another file in the repo, the command also prints where that file is on disk.

It finds five kinds of link:

- normal markdown links, `[text](target)`, including one that wraps onto the next line
- web addresses in angle brackets, `<https://...>`
- wiki links, `[[name]]`
- a file path written as inline code, such as `` `docs/x.md` ``. This is the most common kind in Mavexa, so it is on by default.
- plain web addresses written straight into the text, only when you pass `--bare`

It ignores fenced code blocks, front matter, image links and reference-style links. It does not open or follow any link. It only reports them.

## How it affects a user

- New flags: `--internal` (links to files and headings only), `--external` (web and mailto links only), `--bare`, `--section H`, `--width`, `--max-lines` and `--json`.
- A link to a file in the repo is followed by `-> <path>`, printed relative to where you run the command. When the file does not exist, the line ends in `(missing)`. For a code-span path that is not next to the linking file, mdq looks in each parent directory up to the repo root.
- `#anchor` is split off the target. In JSON it is a separate `anchor` field. In text the target is shown as written.
- Exit code 0 means links were found, 1 means none were found, and 2 means bad arguments, for example `--internal` and `--external` together.
- Every other command behaves exactly as before.

## How to test it

Build it locally with `go build -o mdq .`. Do not use `make install`.

1. `./mdq links ~/workspace/projects/mavexa/mavexa/docs/pages-and-pipeline.md`. Expect a header saying 18 links, and a row at line 234 for `glossary.md#effective-kind` pointing at `…/docs/glossary.md`.
2. `./mdq links testdata/links.md`. Expect rows of every kind except bare URLs, and at least one row ending in `(missing)`.
3. `./mdq links testdata/links.md --bare`. Expect the bare URLs to be listed as well.
4. `./mdq links testdata/links.md --external --json`. Expect a JSON array with web links only.
5. `./mdq links --internal --external testdata/links.md; echo $?`. Expect an error message and `2`.
6. `./mdq links ~/workspace/projects/mavexa/mavexa/docs --internal | grep missing`. This lists broken links across the corpus.

## Build record

| Agent | Package | Model | Turns |
|---|---|---|---|
| builder | links | Sonnet | about 14 (39 tool calls) |

Gates on the merged tip: `go vet` clean and `go build` OK. `go test` passes except `TestRealTodoTasks`. That test fails the same way on the base commit, because the Mavexa todo.md it reads has changed since the test was written, so this build did not cause it.

The builder made these choices where the plan was silent or would not work as written:

- It added `LinksWith(LinkOptions{Bare})` next to `Links()` so that `--bare` reaches the extraction code.
- The JSON field for the heading path is `section_path`, because the resolved file path already uses `path`.
- A code span containing `* ? < > | :` is not treated as a path, so a glob like `docs/*.md` is not reported as a link.
- An existing test asserted an exact count of markdown files under `testdata/`. Adding the new test files broke that count, so it now matches any number.
