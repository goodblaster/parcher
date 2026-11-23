# TODO

- [ ] Optionally downsample embedded images to their displayed dimensions and a
  chosen pixel density. Measure archive size savings and visual quality. Check
  the effect on OCR, especially small text, diagrams and screenshots; consider
  an OCR-friendly minimum resolution or an option to retain original pixels.
- [ ] Support passing login usernames and passwords through CLI parameters or
  environment variables. Define how credentials map to login forms and how
  sessions/profiles are reused. Keep credentials out of logs, reports and
  archives; prefer environment variables over exposing passwords in shell
  history or process arguments.
- [ ] Review the gate workflow before calling it finished: test more common
  sites, assess detection coverage, and settle gate-file lifecycle and CLI
  behavior. Current work is not yet a complete real-site validation.
