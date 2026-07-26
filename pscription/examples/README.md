# rx examples

Each directory is a self-contained example with a `run.sh` you can execute
directly. `run.sh` builds `rx` (to `../../bin/rx`) if needed, then runs the
demo. Most take an optional URL argument.

```bash
examples/article/run.sh                        # default demo URL
examples/article/run.sh https://your.site/post # your own URL
```

| Example | Shows |
|---|---|
| [`article/`](article) | Built-in `@article` extractor — clean article text, LLM-ready. |
| [`metadata/`](metadata) | A **custom user script** (`page-metadata.js`) run against a page; rx captures whatever JSON it returns. |
| [`headings/`](headings) | A **`.rx` pscription file** combining a built-in step (`dismiss-popups`) with a custom script. |
| [`login/`](login) | Interactive login in a visible browser (`--show` + `wait:user`), then extract the page behind it — with a credential-separation pattern for automation. |

## The two things worth learning from these

**Steps** are positional and run in order. A step is one of:

- a **built-in step** — `dismiss-popups`, `wait-quiet`, `scroll-and-wait-images`, …
- a **built-in script** — `@article`, `@links`, `@text`
- **your own script** — `path.js` (optionally `path.js:name` to capture its
  return value under `name`)
- a **wait** — `wait:user` (pause for manual action, needs `--show`) or
  `wait:2s`

**Custom scripts** are just a parenthesized function that returns something
JSON-serializable:

```js
((vars) => ({ title: document.title, links: document.querySelectorAll('a').length }))
```

rx runs it in the page and puts the return value in the report's `outputs`.
The `vars` argument carries values from the pscription (see `login/` for the
credential-separation pattern). `async` functions work too — rx awaits them.

Output is a JSON report on stdout (`--only <name>` prints just one captured
value); logs go to stderr; exit status is non-zero if a step failed.
