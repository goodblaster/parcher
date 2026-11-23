# TODO

- [x] Downsize embedded images to displayed dimensions and capture pixel density
  by default. `--no-downsize-images` retains original pixels;
  `ocr --downsize-images` recognizes first and reduces afterward.
- [ ] Measure image savings and OCR quality across real sites, especially small
  text, diagrams and screenshots. Extend optimization to metadata-bearing images
  and complex CSS backgrounds without changing appearance or animation.
- [ ] Support passing login usernames and passwords through CLI parameters or
  environment variables. Define how credentials map to login forms and how
  sessions/profiles are reused. Keep credentials out of logs, reports and
  archives; prefer environment variables over exposing passwords in shell
  history or process arguments.
- [ ] Review the gate workflow before calling it finished: test more common
  sites, assess detection coverage, and settle gate-file lifecycle and CLI
  behavior. Current work is not yet a complete real-site validation.
