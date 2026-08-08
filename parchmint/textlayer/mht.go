package textlayer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"regexp"
)

// MHT support: in an MHTML archive the layer travels as an extra MIME
// part instead of a <script> element. multipart/related readers render
// only referenced parts, so browsers carry the layer along silently;
// base64 encoding sidesteps quoted-printable entirely.
const (
	// MHTPartType is the Content-Type of the embedded layer part.
	MHTPartType = "application/x-parchmint-text"
	// MHTPartLocation names the part (never referenced by the document).
	MHTPartLocation = "parchmint:text-layer"
)

// IsMHT sniffs archive bytes: an MHT is a MIME message (headers first),
// not markup.
func IsMHT(data []byte) bool {
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	trimmed := bytes.TrimLeft(head, " \t\r\n")
	return !bytes.HasPrefix(trimmed, []byte("<")) &&
		bytes.Contains(head, []byte("multipart/related"))
}

var boundaryRe = regexp.MustCompile(`boundary="([^"]+)"`)

func mhtBoundary(data []byte) ([]byte, error) {
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	m := boundaryRe.FindSubmatch(head)
	if m == nil {
		return nil, fmt.Errorf("mht has no multipart boundary")
	}
	return m[1], nil
}

// mhtCutLayerPart removes an existing layer part (idempotent re-embed).
func mhtCutLayerPart(mht, sep []byte) []byte {
	marker := bytes.Index(mht, []byte("Content-Type: "+MHTPartType))
	if marker == -1 {
		return mht
	}
	start := bytes.LastIndex(mht[:marker], sep)
	rel := bytes.Index(mht[marker:], sep)
	if start == -1 || rel == -1 {
		return mht
	}
	end := marker + rel
	out := make([]byte, 0, len(mht)-(end-start))
	out = append(out, mht[:start]...)
	out = append(out, mht[end:]...)
	return out
}

// EmbedInMHT returns the archive carrying layerJSON as its layer part —
// replacing an existing one — inserted just before the closing boundary.
func EmbedInMHT(mht, layerJSON []byte) ([]byte, error) {
	b, err := mhtBoundary(mht)
	if err != nil {
		return nil, err
	}
	sep := append([]byte("--"), b...)
	mht = mhtCutLayerPart(mht, sep)

	closing := append(append([]byte{}, sep...), '-', '-')
	idx := bytes.LastIndex(mht, closing)
	if idx == -1 {
		return nil, fmt.Errorf("mht has no closing boundary")
	}

	enc := base64.StdEncoding.EncodeToString(layerJSON)
	var part bytes.Buffer
	part.Write(sep)
	part.WriteString("\r\nContent-Type: " + MHTPartType +
		"\r\nContent-Transfer-Encoding: base64" +
		"\r\nContent-Location: " + MHTPartLocation + "\r\n\r\n")
	for len(enc) > 76 {
		part.WriteString(enc[:76])
		part.WriteString("\r\n")
		enc = enc[76:]
	}
	part.WriteString(enc)
	part.WriteString("\r\n")

	out := make([]byte, 0, len(mht)+part.Len())
	out = append(out, mht[:idx]...)
	out = append(out, part.Bytes()...)
	out = append(out, mht[idx:]...)
	return out, nil
}

// mhtLayerBytes extracts the raw layer JSON from the archive's layer part.
func mhtLayerBytes(mht []byte) ([]byte, error) {
	b, err := mhtBoundary(mht)
	if err != nil {
		return nil, err
	}
	marker := bytes.Index(mht, []byte("Content-Type: "+MHTPartType))
	if marker == -1 {
		return nil, fmt.Errorf("no embedded text layer part")
	}
	body, err := mhtPartBody(mht, []byte("--"+string(b)), marker)
	if err != nil {
		return nil, err
	}
	clean := bytes.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, body)
	out := make([]byte, base64.StdEncoding.DecodedLen(len(clean)))
	n, err := base64.StdEncoding.Decode(out, clean)
	if err != nil {
		return nil, fmt.Errorf("text layer part is not valid base64: %w", err)
	}
	return out[:n], nil
}

// mhtPartBody returns the body of the part whose headers contain the
// given marker offset: from the blank line after the headers to the next
// boundary.
func mhtPartBody(mht, sep []byte, marker int) ([]byte, error) {
	bodyStart := bytes.Index(mht[marker:], []byte("\r\n\r\n"))
	if bodyStart == -1 {
		return nil, fmt.Errorf("malformed mht part headers")
	}
	bodyStart += marker + 4
	rel := bytes.Index(mht[bodyStart:], sep)
	if rel == -1 {
		return nil, fmt.Errorf("unterminated mht part")
	}
	return mht[bodyStart : bodyStart+rel], nil
}

// ReplaceMHTDocument returns the archive with its primary text/html part
// replaced by doc (quoted-printable encoded, like Chrome writes it).
// Every other part — images, stylesheets, frames, the text layer — is
// kept, so cid: references inside doc keep resolving. This is how a
// marked copy of an MHT stays an MHT.
func ReplaceMHTDocument(mht, doc []byte) ([]byte, error) {
	b, err := mhtBoundary(mht)
	if err != nil {
		return nil, err
	}
	sep := []byte("--" + string(b))
	marker := bytes.Index(mht, []byte("Content-Type: text/html"))
	if marker == -1 {
		return nil, fmt.Errorf("mht has no text/html part")
	}
	partStart := bytes.LastIndex(mht[:marker], sep)
	headEnd := bytes.Index(mht[marker:], []byte("\r\n\r\n"))
	if partStart == -1 || headEnd == -1 {
		return nil, fmt.Errorf("malformed mht part")
	}
	bodyStart := marker + headEnd + 4
	rel := bytes.Index(mht[bodyStart:], sep)
	if rel == -1 {
		return nil, fmt.Errorf("unterminated mht part")
	}
	bodyEnd := bodyStart + rel

	// Keep the part's headers, forcing the encoding we actually write.
	headers := append([]byte{}, mht[partStart:bodyStart]...)
	for _, enc := range []string{"base64", "binary", "7bit", "8bit"} {
		headers = bytes.Replace(headers,
			[]byte("Content-Transfer-Encoding: "+enc),
			[]byte("Content-Transfer-Encoding: quoted-printable"), 1)
	}

	var body bytes.Buffer
	qp := quotedprintable.NewWriter(&body)
	if _, err := qp.Write(doc); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}

	out := make([]byte, 0, partStart+len(headers)+body.Len()+2+len(mht)-bodyEnd)
	out = append(out, mht[:partStart]...)
	out = append(out, headers...)
	out = append(out, body.Bytes()...)
	out = append(out, '\r', '\n')
	out = append(out, mht[bodyEnd:]...)
	return out, nil
}

// MHTDocument returns the decoded primary text/html part of the archive —
// the same markup an HTML archive holds directly, with data URIs intact.
// Index-time image pairing scans this.
func MHTDocument(mht []byte) ([]byte, error) {
	b, err := mhtBoundary(mht)
	if err != nil {
		return nil, err
	}
	sep := []byte("--" + string(b))
	marker := bytes.Index(mht, []byte("Content-Type: text/html"))
	if marker == -1 {
		return nil, fmt.Errorf("mht has no text/html part")
	}
	body, err := mhtPartBody(mht, sep, marker)
	if err != nil {
		return nil, err
	}
	// Chrome encodes the document part quoted-printable; tolerate plain
	// or base64 too.
	headers := mht[marker:]
	if end := bytes.Index(headers, []byte("\r\n\r\n")); end != -1 {
		headers = headers[:end]
	}
	switch {
	case bytes.Contains(headers, []byte("quoted-printable")):
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
	case bytes.Contains(headers, []byte("base64")):
		out := make([]byte, base64.StdEncoding.DecodedLen(len(body)))
		n, err := base64.StdEncoding.Decode(out, bytes.ReplaceAll(body, []byte("\r\n"), nil))
		if err != nil {
			return nil, err
		}
		return out[:n], nil
	default:
		return body, nil
	}
}
