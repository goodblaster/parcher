package pack

import (
	"io"
	"strings"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/convert/css"
	"github.com/goodblaster/parchmint/internal/convert/refs"
	"golang.org/x/net/html"
)

// scanHTML walks source, collecting every subresource it references and
// rewriting the inline data: URIs into cid: references. The returned markup
// is otherwise byte-for-byte the input: only the elements that changed are
// re-serialized.
func (p *packer) scanHTML(source string, base target) (string, []target) {
	var (
		out     strings.Builder
		pending []target
		inStyle bool
		inTitle bool
	)
	out.Grow(len(source))

	tokenizer := html.NewTokenizer(strings.NewReader(source))

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizer.Err(); !errors.Is(err, io.EOF) {
				p.warn("HTML: %v", err)
			}
			return out.String(), pending

		case html.TextToken:
			raw := string(tokenizer.Raw())
			switch {
			case inStyle:
				out.WriteString(css.Rewrite(raw, func(ref string) string {
					return p.reference(ref, base, &pending)
				}))
			case inTitle && p.title == "":
				// The first <title> seen names the page: the root document
				// is scanned before any frame it contains.
				p.title = html.UnescapeString(raw)
				out.WriteString(raw)
			default:
				out.WriteString(raw)
			}

		case html.StartTagToken, html.SelfClosingTagToken:
			raw := string(tokenizer.Raw())
			token := tokenizer.Token()

			switch token.Data {
			case "style":
				inStyle = true
			case "title":
				inTitle = true
			case "base":
				// A <base href> moves the ground both resolutions stand on.
				if href, ok := attr(token, "href"); ok {
					if resolved := resolve(href, base.url); resolved != nil {
						base.url = resolved
					}
					if base.fallback != nil {
						if resolved := resolve(href, base.fallback); resolved != nil {
							base.fallback = resolved
						}
					}
				}
			}

			if p.rewriteAttrs(&token, base, &pending) {
				out.WriteString(token.String())
				continue
			}
			out.WriteString(raw)

		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			switch string(name) {
			case "style":
				inStyle = false
			case "title":
				inTitle = false
			}
			out.Write(tokenizer.Raw())

		default:
			out.Write(tokenizer.Raw())
		}
	}
}

// scanCSS collects the references in a stylesheet, rewriting only its inline
// data: URIs. Ordinary references stay as written: the stylesheet becomes a
// part at its own URL, so they keep resolving.
func (p *packer) scanCSS(source string, base target) (string, []target) {
	var pending []target

	rewritten := css.Rewrite(source, func(ref string) string {
		return p.reference(ref, base, &pending)
	})

	return rewritten, pending
}

// rewriteAttrs handles every subresource attribute on token, reporting
// whether any of their values changed.
func (p *packer) rewriteAttrs(token *html.Token, base target, pending *[]target) bool {
	changed := false

	for i := range token.Attr {
		attribute := &token.Attr[i]
		key := strings.ToLower(attribute.Key)

		if key == "style" {
			rewritten := css.Rewrite(attribute.Val, func(ref string) string {
				return p.reference(ref, base, pending)
			})
			if rewritten != attribute.Val {
				attribute.Val = rewritten
				changed = true
			}
			continue
		}

		if !isResourceAttr(*token, key) {
			continue
		}

		if refs.IsSrcset(key) {
			candidates := refs.ParseSrcset(attribute.Val)
			for j := range candidates {
				candidates[j].URL = p.reference(candidates[j].URL, base, pending)
			}
			if rewritten := refs.FormatSrcset(candidates); rewritten != attribute.Val {
				attribute.Val = rewritten
				changed = true
			}
			continue
		}

		if rewritten := p.reference(attribute.Val, base, pending); rewritten != attribute.Val {
			attribute.Val = rewritten
			changed = true
		}
	}

	return changed
}

// isResourceAttr reports whether key on token names a subresource. The href
// of a <link> counts only for the relations that actually load content.
func isResourceAttr(token html.Token, key string) bool {
	for _, candidate := range refs.Attrs(token.Data) {
		if candidate != key {
			continue
		}
		if token.Data == "link" && key == "href" {
			rel, _ := attr(token, "rel")
			return refs.IsResourceLink(rel)
		}
		return true
	}
	return false
}

// attr returns the value of the named attribute on token.
func attr(token html.Token, name string) (string, bool) {
	for _, a := range token.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val, true
		}
	}
	return "", false
}
