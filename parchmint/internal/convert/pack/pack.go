// Package pack builds an MHT document from an HTML page and the resources it
// references: relative and file: references are read from disk, and http(s)
// references are fetched when the network is enabled. Inline data: URIs are
// already self-contained and stay exactly where they are.
//
// Resources are discovered breadth-first: the page yields its references,
// each fetched stylesheet yields the images and fonts it imports, and each
// fetched frame yields a page of its own. References that cannot be resolved
// are left in place, so a partial snapshot still carries what it could get.
package pack

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/go-mht/pkg/documents"
	"github.com/goodblaster/go-mht/pkg/sections"
	"github.com/goodblaster/parcher/parchmint/internal/convert/fetch"
	"github.com/goodblaster/parcher/parchmint/internal/convert/refs"
)

// Defaults applied to a zero-valued Options.
const (
	DefaultConcurrency = 8
	DefaultTimeout     = 30 * time.Second
	DefaultFrom        = "<Saved by go-mht>"
)

// Options controls how a page is packed. The zero value packs offline: local
// files only, with nothing fetched over the network.
type Options struct {
	// Base is the URL the page is treated as living at. Relative references
	// resolve against it, and it becomes the document's
	// Snapshot-Content-Location. File defaults it to the input's file: URL.
	Base string

	// FallbackBase is the address the page actually came from, used to
	// retrieve what Base cannot. A saved page can reference a resource that
	// escapes the directory it was saved into (../../content/dam/x.svg):
	// against Base that resolves to somewhere on the filesystem where nothing
	// was ever written, and only the original address can supply it.
	//
	// The reference is re-resolved from scratch against this base — rewriting
	// the failed file: URL would not do, since the relative path is gone by
	// then. Retries are network fetches, so they need Network. Empty disables
	// the fallback.
	FallbackBase string

	// Subject overrides the document subject, which otherwise comes from the
	// page's <title>.
	Subject string

	// From is the document's From header. Empty means DefaultFrom.
	From string

	// Date is the document date. The zero time means now.
	Date time.Time

	// Network enables fetching http(s) references. Without it, those
	// references are left in place and reported through OnWarn.
	Network bool

	// Client overrides the HTTP client used when Network is set.
	Client *http.Client

	// Timeout bounds each individual resource fetch. Zero means
	// DefaultTimeout.
	Timeout time.Duration

	// MaxSize caps the size of any one resource. Zero means
	// fetch.DefaultMaxSize; negative means no limit.
	MaxSize int64

	// Concurrency bounds simultaneous fetches. Zero means
	// DefaultConcurrency.
	Concurrency int

	// OnWarn, when set, is called for each resource that could not be
	// included, with a short explanation.
	OnWarn func(message string)
}

// HTML packs source, an HTML page, into an MHT document.
func HTML(ctx context.Context, source []byte, opts Options) (*documents.Document, error) {
	base := parseURL(opts.Base)
	if opts.Base != "" && base == nil {
		return nil, errors.Newf("invalid base URL %q", opts.Base)
	}

	fallback := parseURL(opts.FallbackBase)
	if opts.FallbackBase != "" && fallback == nil {
		return nil, errors.Newf("invalid fallback base URL %q", opts.FallbackBase)
	}

	return newPacker(ctx, opts).run(string(source), target{url: base, fallback: fallback})
}

// target is one resource to retrieve: where it should be, and where else to
// look when it is not there.
type target struct {
	// url is the resource's address, resolved against the document holding
	// the reference. It also names the resulting part.
	url *url.URL

	// fallback is the same reference resolved against the page's original
	// address, tried when url yields nothing. Nil when there is no second
	// place to look.
	fallback *url.URL
}

// File packs the named HTML file, resolving its relative references against
// the file's own location unless Options.Base says otherwise.
func File(ctx context.Context, name string, opts Options) (*documents.Document, error) {
	source, err := os.ReadFile(name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}

	if opts.Base == "" {
		absolute, err := filepath.Abs(name)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve path")
		}
		opts.Base = (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
	}

	return HTML(ctx, source, opts)
}

// part is one section-to-be: the bytes of a resource plus the headers that
// will let a viewer find it again.
type part struct {
	location    string
	contentID   string
	contentType string
	data        []byte
}

type packer struct {
	ctx    context.Context
	opts   Options
	loader *fetch.Loader

	// seen holds every absolute URL already queued, so a resource shared by
	// many references is fetched once.
	seen map[string]bool

	parts  []*part
	frames int
	title  string
}

func newPacker(ctx context.Context, opts Options) *packer {
	loader := &fetch.Loader{MaxSize: opts.MaxSize}
	if opts.Network {
		loader.Client = opts.Client
		if loader.Client == nil {
			loader.Client = &http.Client{}
		}
	}

	return &packer{
		ctx:    ctx,
		opts:   opts,
		loader: loader,
		seen:   make(map[string]bool),
	}
}

func (p *packer) run(source string, base target) (*documents.Document, error) {
	root := &part{
		location:    urlString(base.url),
		contentID:   p.frameID(),
		contentType: "text/html",
	}

	// The page is already a part, so a frame or link that points back at it
	// must not pull it in a second time.
	if base.url != nil {
		rootURL := *base.url
		rootURL.Fragment = ""
		p.seen[rootURL.String()] = true
	}

	rewritten, pending := p.scanHTML(source, base)
	root.data = []byte(rewritten)

	// Breadth-first: each wave of fetches can reveal the next.
	for len(pending) > 0 {
		pending = p.wave(pending)
	}

	return p.document(root)
}

// wave fetches every URL in pending concurrently and returns the references
// those resources brought with them. Results are processed in the order the
// URLs were discovered, so the output does not depend on scheduling.
func (p *packer) wave(pending []target) []target {
	concurrency := p.opts.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	timeout := p.opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	resources := make([]*fetch.Resource, len(pending))
	loaded := make([]*url.URL, len(pending))
	failures := make([]error, len(pending))

	var group sync.WaitGroup
	tokens := make(chan struct{}, concurrency)

	for i, t := range pending {
		group.Add(1)
		go func() {
			defer group.Done()

			tokens <- struct{}{}
			defer func() { <-tokens }()

			ctx, cancel := context.WithTimeout(p.ctx, timeout)
			defer cancel()

			resources[i], loaded[i], failures[i] = p.load(ctx, t)
		}()
	}
	group.Wait()

	var next []target
	for i, t := range pending {
		if failures[i] != nil {
			p.warn("%s: %v", t.url, failures[i])
			continue
		}
		next = append(next, p.add(t, loaded[i], resources[i])...)
	}

	return next
}

// load retrieves one target, trying its fallback when the first address
// yields nothing. It returns the resource and the address that supplied it,
// which is what the resulting part gets named after.
//
// A failed fallback is reported as the ORIGINAL failure: the first address is
// where the resource was supposed to be, and saying "no such file" is more use
// than a connection error against a URL the caller never wrote.
func (p *packer) load(ctx context.Context, t target) (*fetch.Resource, *url.URL, error) {
	resource, err := p.loader.Load(ctx, t.url)
	if err == nil {
		return resource, t.url, nil
	}
	if t.fallback == nil || !p.opts.Network {
		return nil, nil, err
	}

	resource, fallbackErr := p.loader.Load(ctx, t.fallback)
	if fallbackErr != nil {
		return nil, nil, err
	}

	return resource, t.fallback, nil
}

// add turns a fetched resource into a part, descending into stylesheets and
// frames to collect the references they carry.
func (p *packer) add(t target, from *url.URL, resource *fetch.Resource) []target {
	newPart := &part{
		location:    from.String(),
		contentType: resource.ContentType,
		data:        resource.Data,
	}

	// References inside this resource resolve against wherever it lives. The
	// fallback carries down only if the resource came from its first address:
	// once retrieved from the fallback, that IS its address and there is
	// nowhere further to fall back to.
	base := target{url: from}
	if from == t.url {
		base.fallback = t.fallback
	}

	var pending []target
	switch {
	case isCSS(resource.ContentType):
		rewritten, refs := p.scanCSS(string(resource.Data), base)
		newPart.data = []byte(rewritten)
		pending = refs
	case isHTML(resource.ContentType):
		newPart.contentID = p.frameID()
		rewritten, refs := p.scanHTML(string(resource.Data), base)
		newPart.data = []byte(rewritten)
		pending = refs
	}

	p.parts = append(p.parts, newPart)

	return pending
}

// enqueue resolves ref against base and returns the target to retrieve, or
// nil when the reference is not something to include.
func (p *packer) enqueue(ref string, base target) *target {
	primary := resolve(ref, base.url)
	if primary == nil {
		p.warn("cannot resolve %s", ref)
		return nil
	}

	switch strings.ToLower(primary.Scheme) {
	case "file", "http", "https":
	default:
		return nil
	}
	if !p.opts.Network && isNetwork(primary) {
		p.warn("%s: %v", primary, fetch.ErrOffline)
		return nil
	}

	// Fragments identify a position within a resource, not a resource.
	primary.Fragment = ""

	// The same reference against the page's own address, for when the first
	// one turns up nothing. Only a network address is worth a second try —
	// a second look at the filesystem would find the same nothing.
	var fallback *url.URL
	if base.fallback != nil {
		if resolved := resolve(ref, base.fallback); resolved != nil && isNetwork(resolved) {
			resolved.Fragment = ""
			if resolved.String() != primary.String() {
				fallback = resolved
			}
		}
	}

	key := primary.String()
	if p.seen[key] {
		return nil
	}
	p.seen[key] = true

	return &target{url: primary, fallback: fallback}
}

func isNetwork(target *url.URL) bool {
	scheme := strings.ToLower(target.Scheme)
	return scheme == "http" || scheme == "https"
}

// reference handles one reference found in the page: inline data becomes a
// part immediately, anything else is queued for fetching and left as written
// so that it still resolves against the part's own location.
func (p *packer) reference(ref string, base target, pending *[]target) string {
	if !refs.Fetchable(ref) {
		return ref
	}

	trimmed := strings.ToLower(strings.TrimSpace(ref))

	// A data: URI is already self-contained, so it is left exactly where it
	// is. Moving one into a part and referring back to it by cid: looks
	// tidier — it deduplicates repeats — but Chrome does not resolve those
	// references, and every inline image in the archive renders as a broken
	// placeholder. An inline data: URI always renders; that is worth more
	// than the deduplication.
	if strings.HasPrefix(trimmed, "data:") {
		return ref
	}
	// A cid: reference already names a part; there is nothing to fetch.
	if strings.HasPrefix(trimmed, "cid:") {
		return ref
	}
	if enqueued := p.enqueue(ref, base); enqueued != nil {
		*pending = append(*pending, *enqueued)
	}

	return ref
}

// document assembles the parsed parts into a Document with Blink-compatible
// headers.
func (p *packer) document(root *part) (*documents.Document, error) {
	boundary, err := makeBoundary()
	if err != nil {
		return nil, err
	}

	subject := p.opts.Subject
	if subject == "" {
		subject = p.title
	}

	date := p.opts.Date
	if date.IsZero() {
		date = time.Now()
	}

	from := p.opts.From
	if from == "" {
		from = DefaultFrom
	}

	doc := documents.New()
	doc.Boundary = boundary
	doc.Headers.Set("From", from)
	if root.location != "" {
		doc.Headers.Set("Snapshot-Content-Location", root.location)
	}
	if subject != "" {
		doc.Headers.Set("Subject", encodeSubject(subject))
	}
	doc.Headers.Set("Date", date.Format("Mon, 2 Jan 2006 15:04:05 -0700"))
	doc.Headers.Set("MIME-Version", "1.0")
	doc.Headers.Set("Content-Type", `multipart/related; type="text/html"; boundary="`+boundary+`"`)

	for _, current := range append([]*part{root}, p.parts...) {
		doc.Sections = append(doc.Sections, current.section())
	}
	for i := range doc.Sections {
		if location := doc.Sections[i].Headers.Get("Content-Location"); location != "" {
			doc.LocIndex[location] = &doc.Sections[i]
		}
	}

	return doc, nil
}

// section renders a part as an MHT section, choosing quoted-printable for
// text and base64 for everything else, as Blink does.
func (p *part) section() sections.Section {
	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Type", p.contentType)

	if p.contentID != "" {
		headers.Set("Content-ID", p.contentID)
	}

	section := sections.Section{Headers: headers}

	if isText(p.contentType) {
		headers.Set("Content-Transfer-Encoding", "quoted-printable")
		// The body ends with exactly one line terminator. The line break that
		// separates a part from the next boundary is NOT content: go-mht
		// writes it after the body and strips it again on parse (since
		// v0.0.2 — before that a parsed body carried it, and this code had to
		// supply it; doing so now leaves a stray blank line in every text
		// part).
		//
		// Trailing newlines are normalized to that one terminator rather
		// than added to whatever is already there: a body that has been
		// through this once already must come out of it unchanged, or
		// repeated pack/render cycles would grow the content each time.
		section.Body = strings.TrimRight(string(p.data), "\r\n") + "\r\n"
	} else {
		headers.Set("Content-Transfer-Encoding", "base64")
		section.Body = base64.StdEncoding.EncodeToString(p.data)
	}

	if p.location != "" {
		headers.Set("Content-Location", p.location)
	}

	return section
}

func (p *packer) frameID() string {
	p.frames++
	return fmt.Sprintf("<frame-%04d@mhtml.goodblaster>", p.frames)
}

func (p *packer) warn(format string, args ...any) {
	if p.opts.OnWarn != nil {
		p.opts.OnWarn(fmt.Sprintf(format, args...))
	}
}

// makeBoundary generates a multipart boundary in Blink's shape.
func makeBoundary() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	buf := make([]byte, 42)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.Wrap(err, "failed to generate boundary")
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}

	return "----MultipartBoundary--" + string(buf) + "----", nil
}

// encodeSubject encodes a subject for a header, using RFC 2047 only when the
// title is not plain ASCII.
func encodeSubject(subject string) string {
	subject = strings.Join(strings.Fields(subject), " ")
	for i := range len(subject) {
		if subject[i] >= 0x80 {
			return mime.QEncoding.Encode("utf-8", subject)
		}
	}
	return subject
}

func isText(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if index := strings.IndexByte(mediaType, ';'); index >= 0 {
		mediaType = strings.TrimSpace(mediaType[:index])
	}

	if strings.HasPrefix(mediaType, "text/") || strings.HasSuffix(mediaType, "+xml") ||
		strings.HasSuffix(mediaType, "+json") {
		return true
	}

	switch mediaType {
	case "application/javascript", "application/x-javascript", "application/ecmascript",
		"application/json", "application/xml", "image/svg+xml":
		return true
	}

	return false
}

func isCSS(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "text/css")
}

func isHTML(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.HasPrefix(contentType, "text/html") ||
		strings.HasPrefix(contentType, "application/xhtml+xml")
}

func parseURL(raw string) *url.URL {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return parsed
}

func urlString(target *url.URL) string {
	if target == nil {
		return ""
	}
	return target.String()
}

// resolve turns ref into an absolute URL against base. A protocol-relative
// reference under a non-web base is given https, since "file://cdn/x.png"
// would be meaningless.
func resolve(ref string, base *url.URL) *url.URL {
	ref = strings.TrimSpace(ref)

	if strings.HasPrefix(ref, "//") && (base == nil || (base.Scheme != "http" && base.Scheme != "https")) {
		ref = "https:" + ref
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		return nil
	}
	if parsed.IsAbs() {
		return parsed
	}
	if base == nil {
		return nil
	}

	return base.ResolveReference(parsed)
}
