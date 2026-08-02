// Package pack builds an MHT document from an HTML page and the resources it
// references. Inline data: URIs become parts of their own, relative and
// file: references are read from disk, and http(s) references are fetched
// when the network is enabled.
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
	"github.com/goodblaster/parchmint/internal/convert/fetch"
	"github.com/goodblaster/parchmint/internal/convert/refs"
)

// Defaults applied to a zero-valued Options.
const (
	DefaultConcurrency = 8
	DefaultTimeout     = 30 * time.Second
	DefaultFrom        = "<Saved by go-mht>"
)

// Options controls how a page is packed. The zero value packs offline: data:
// URIs and local files only.
type Options struct {
	// Base is the URL the page is treated as living at. Relative references
	// resolve against it, and it becomes the document's
	// Snapshot-Content-Location. File defaults it to the input's file: URL.
	Base string

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

	return newPacker(ctx, opts).run(string(source), base)
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

	// dataRefs maps an inline data: URI to the cid: reference that replaced
	// it, so repeats collapse onto one part.
	dataRefs map[string]string

	parts  []*part
	frames int
	blobs  int
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
		ctx:      ctx,
		opts:     opts,
		loader:   loader,
		seen:     make(map[string]bool),
		dataRefs: make(map[string]string),
	}
}

func (p *packer) run(source string, base *url.URL) (*documents.Document, error) {
	root := &part{
		location:    urlString(base),
		contentID:   p.frameID(),
		contentType: "text/html",
	}

	// The page is already a part, so a frame or link that points back at it
	// must not pull it in a second time.
	if base != nil {
		rootURL := *base
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
func (p *packer) wave(pending []*url.URL) []*url.URL {
	concurrency := p.opts.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	timeout := p.opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	resources := make([]*fetch.Resource, len(pending))
	failures := make([]error, len(pending))

	var group sync.WaitGroup
	tokens := make(chan struct{}, concurrency)

	for i, target := range pending {
		group.Add(1)
		go func() {
			defer group.Done()

			tokens <- struct{}{}
			defer func() { <-tokens }()

			ctx, cancel := context.WithTimeout(p.ctx, timeout)
			defer cancel()

			resources[i], failures[i] = p.loader.Load(ctx, target)
		}()
	}
	group.Wait()

	var next []*url.URL
	for i, target := range pending {
		if failures[i] != nil {
			p.warn("%s: %v", target, failures[i])
			continue
		}
		next = append(next, p.add(target, resources[i])...)
	}

	return next
}

// add turns a fetched resource into a part, descending into stylesheets and
// frames to collect the references they carry.
func (p *packer) add(target *url.URL, resource *fetch.Resource) []*url.URL {
	newPart := &part{
		location:    target.String(),
		contentType: resource.ContentType,
		data:        resource.Data,
	}

	var pending []*url.URL
	switch {
	case isCSS(resource.ContentType):
		rewritten, refs := p.scanCSS(string(resource.Data), target)
		newPart.data = []byte(rewritten)
		pending = refs
	case isHTML(resource.ContentType):
		newPart.contentID = p.frameID()
		rewritten, refs := p.scanHTML(string(resource.Data), target)
		newPart.data = []byte(rewritten)
		pending = refs
	}

	p.parts = append(p.parts, newPart)

	return pending
}

// enqueue resolves ref against base and returns the absolute URL to fetch,
// or nil when the reference is not something to include.
func (p *packer) enqueue(ref string, base *url.URL) *url.URL {
	target := resolve(ref, base)
	if target == nil {
		p.warn("cannot resolve %s", ref)
		return nil
	}

	switch strings.ToLower(target.Scheme) {
	case "file", "http", "https":
	default:
		return nil
	}
	if !p.opts.Network && (target.Scheme == "http" || target.Scheme == "https") {
		p.warn("%s: %v", target, fetch.ErrOffline)
		return nil
	}

	// Fragments identify a position within a resource, not a resource.
	target.Fragment = ""

	key := target.String()
	if p.seen[key] {
		return nil
	}
	p.seen[key] = true

	return target
}

// inlineData converts a data: URI into a part of its own and returns the
// cid: reference that replaces it. On failure it returns the URI unchanged.
func (p *packer) inlineData(ref string) string {
	if existing, ok := p.dataRefs[ref]; ok {
		return existing
	}

	target, err := url.Parse(ref)
	if err != nil {
		p.warn("malformed data URI: %v", err)
		return ref
	}

	resource, err := p.loader.Load(p.ctx, target)
	if err != nil {
		p.warn("data URI: %v", err)
		return ref
	}

	p.blobs++
	contentID := fmt.Sprintf("<data-%04d@mhtml.goodblaster>", p.blobs)

	p.parts = append(p.parts, &part{
		contentID:   contentID,
		contentType: resource.ContentType,
		data:        resource.Data,
	})

	replacement := "cid:" + strings.Trim(contentID, "<>")
	p.dataRefs[ref] = replacement

	return replacement
}

// reference handles one reference found in the page: inline data becomes a
// part immediately, anything else is queued for fetching and left as written
// so that it still resolves against the part's own location.
func (p *packer) reference(ref string, base *url.URL, pending *[]*url.URL) string {
	if !refs.Fetchable(ref) {
		return ref
	}

	trimmed := strings.ToLower(strings.TrimSpace(ref))
	if strings.HasPrefix(trimmed, "data:") {
		return p.inlineData(ref)
	}
	// A cid: reference already names a part; there is nothing to fetch.
	if strings.HasPrefix(trimmed, "cid:") {
		return ref
	}
	if target := p.enqueue(ref, base); target != nil {
		*pending = append(*pending, target)
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
		// The body has to end with a line terminator plus the blank line
		// that separates it from the next boundary. A parsed section carries
		// both in its decoded body, so a synthesized one must supply them —
		// without the blank line the boundary is appended to the last line
		// of content and the document no longer parses.
		//
		// Trailing newlines are normalized to exactly that pair rather than
		// added to whatever is already there: a body that has been through
		// this once already must come out of it unchanged, or repeated
		// pack/render cycles would grow the content two bytes at a time.
		section.Body = strings.TrimRight(string(p.data), "\r\n") + "\r\n\r\n"
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
