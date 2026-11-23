# Access gates and scriptable captures

A capture can succeed as a file while still showing an access decision. By
default, `parch` preserves detected age/research declarations, login forms and
other supported gates. It saves a **gated archive and matching capture metadata file**, then
exits **3**. It does not implicitly assert eligibility or press Escape on a
detected access gate.

## Capture, then clear

```bash
bin/parch -o page.html https://example.com
# If a gate remains, saves page.gated.html and page.json; exits 3.
# Without a gate, saves page.html; exits 0.

bin/parch --clear page.json
# Uses the URL, gate fingerprints, format and regular output name from that file.
# Successful clearance saves page.html; exits 0.
```

The gate file contains the displayed statements, controls, terms links and
submission action. Calling `--clear` authorizes those recorded decisions. It
checks the actual required checkboxes and clicks the normal entry button, then
verifies that the gate disappears. Changed declarations or a new gate require
a fresh decision: capture stops, updates the gated artifact pair, and exits 3.
The regular output is not written while a gate remains.

`--clear FILE` needs no positional URL. If a URL is also supplied, it must match
the file. Other capture flags can be supplied normally: `-o` overrides the saved
regular name, and `-f` overrides the saved format (updating the default output
extension). Other settings use the current configuration/defaults; repeat flags
such as `--profile` and `--width` when they matter to the original capture.

The original gated archive and gate file remain after successful clearance,
so the pair documents the initial view and the gate file can be reused. Their
continued existence does not mean a later capture is gated: use exit status 3
to identify a newly saved unresolved capture. An invocation that fails may also
leave older artifacts in place.

## Names and destinations

For a regular destination `captures/page.html`:

| Artifact | Filename |
| --- | --- |
| Unresolved archive | `captures/page.gated.html` |
| Capture metadata | `captures/page.json` |
| Cleared or non-gated archive | `captures/page.html` |

The metadata filename uses the regular archive stem: `page.gated.html`,
`page.gated.mht` and `page.gated.pdf` all map to `page.json`. It describes the
latest gated capture for that stem. Use distinct stems if you need to retain
separate metadata for several formats in the same directory.

The `.gated` marker is inserted before the final extension, or appended to a
filename without an extension; an existing marker is not duplicated. These
rules apply to explicit `-o` paths, configured filename templates and normal
URL-derived names. There is no automatic `.cleared` suffix.

Gate files store output **basenames relative to their own directory**. Move the
artifact pair together and `--clear` writes the regular output beside them.
The JSON is general capture metadata: access decisions live under `access`,
leaving room for other information in the future. It is currently created
when unresolved gates remain.

`-o -` and implicit piped stdout still stream archive bytes. If unresolved gates
remain, parch also saves the mandatory gated artifact pair in the current
directory under the URL-derived name. It cannot discover the destination of a
shell redirection. Scripts that need exact artifact names should pass `-o`.
Logs and saved gate-file paths go to stderr.

## Other gate options

- `--gates=preserve` deliberately retains a detected gate and writes the same
  artifact pair, but exits 0 on success.
- `--approve-gate ID` approves an exact fingerprint directly (repeatable).
- `--auto-clear` approves supported gates as they appear, including their
  declarations and terms, in one browser session. Successful output uses the
  regular name. It attempts at most eight submissions per capture. If a gate
  remains, the usual gated artifact pair is saved and exit status is 3.

`--clear` cannot be combined with `--auto-clear`, `--approve-gate`, or
`--gates=preserve`. Preservation cannot be combined with an approval mode.

Approval supports unambiguous entry buttons and simple required checkbox forms.
Credentials, dates of birth, custom controls and detected CAPTCHAs require manual
interaction. An authenticated `--profile` may already have cleared a gate;
preservation does not erase that state. Omit `--profile` for a fresh session.

## Gate-file and report contracts

The automatic capture metadata file uses schema **`parch-metadata/1`**. It contains:

- `url`, `final_url`: requested and observed URLs.
- `output`: gated archive basename, such as `page.gated.html`.
- `clear_output`: regular output basename, such as `page.html`.
- `format`, `mime`: capture format and MIME type.
- `access`: the observed access outcome and gate evidence.

`access.status` is `decision_required` or `preserved` in a gate file.
`access.pending` contains the remaining decisions; `access.cleared` records
previously cleared gates. Each decision includes `id`, `kind`, `text`, `controls`,
`links`, `url`, `action`, `method`, `button`, `approvable`, and a pending `reason`.
`access.auto_clear: true` identifies an automatic-approval run.

`--clear` also accepts existing `parch-gate/1` files, including their older
`.gate.json` filenames. New captures write the shorter metadata filename.

`--clear` validates the schema, output names and fingerprints before launching
a browser. A fingerprint covers the displayed declarations, control labels,
links, submission action and page URL (except its fragment). It excludes hidden
form tokens, checked state and transient `role=status` / `role=alert` feedback.
The linked terms' URLs are covered, not the contents of linked pages. Evidence
is checked again before interaction; editing the evidence without its matching
fingerprint is an error.

Pending reasons include `approval_required`, `manual_interaction_required`,
`gate_changed_before_submission`, `acceptance_not_confirmed`, and
`auto_clear_limit_reached`. No gate is submitted repeatedly after a failed
attempt. Reports can contain private page text.

The optional `--report FILE` still writes a **`parch-capture/1`** report for any
successful capture, including pages without gates. Its `output` is the actual
archive path (`-` for stdout without a gated disk copy); it is separate from the
automatic gate file and cannot replace it. HTML and MHT also embed the capture
outcome as inert `application/x-parch-capture+json` metadata, without an output
filename. Capture outcomes additionally include `no_gate_detected` and `cleared`.

## Exit status and Bash

| Code | Meaning for capture |
| --- | --- |
| 0 | Capture completed, or preservation explicitly requested |
| 1 | Capture or artifact/report write failed |
| 2 | Invalid invocation or invalid `--clear` file |
| 3 | Gated archive and matching gate file saved; a decision remains |

These meanings apply to capture, not the grep-style statuses of `parch find`.
A script can inspect the fresh gate file, decide whether its policy permits
clearance, and invoke `--clear`. This example uses an explicit opt-in from the
caller; it also handles exit 3 under `set -e`:

```bash
#!/usr/bin/env bash
set -euo pipefail
url=${1:?usage: capture.sh URL}
status=0
bin/parch -o page.html "$url" || status=$?
case "$status" in
  0) exit 0 ;;
  3) test -f page.json ;;
  *) exit "$status" ;;
esac
jq '.access.pending' page.json
if [[ ${CLEAR_GATES:-0} == 1 ]]; then
  bin/parch --clear page.json
else
  exit 3
fi
```

A subsequent exit 3 means the updated gate file needs another decision, not an
instruction to retry indefinitely. Each invocation writes one archive, plus a
matching gate file when unresolved (and an optional separate capture report).

## Detection limits and remaining work

Detection combines visible dialog/form structure with access language; ordinary
footer disclaimers and purchase confirmation forms are not entry gates. Checks
run before prep, between steps and before capture. Cleanup also checks for gates
revealed by its own clicks. Age-verification cookies and storage values are no
longer preseeded; cookie-banner preparation otherwise retains its behavior.

Detection is heuristic, initially focused on English DOM gates in the main
document and open shadow roots. Closed shadow roots, embedded-frame content,
unusual controls/languages, native dialogs and server-side substitute pages can
need further work. Named visible CAPTCHA frames can be reported, never solved.
`no_gate_detected` is not proof of unrestricted content. Custom recipes execute
arbitrary actions; detection is not a security boundary around them. `--strict`
checks HTML resource embedding independently of gate status.

Local browser fixtures cover detection, preservation, approvals, gate-file
round trips, changed declarations, successive gates, late arrivals, failed
submissions, shadow DOM and CLI outcomes. Broader real-site testing and final
design review remain on the repository [TODO list](../TODO.md).
