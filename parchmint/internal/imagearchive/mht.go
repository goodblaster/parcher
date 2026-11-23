package imagearchive

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"

	"github.com/goodblaster/parcher/parchmint/textlayer"
)

// MapDocuments edits HTML, including same-origin frame documents in MHTML.
// Other MIME parts retain their exact encoded bodies and headers.
func MapDocuments(data []byte, edit func([]byte) []byte) ([]byte, error) {
	if !textlayer.IsMHT(data) {
		return edit(data), nil
	}
	message, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	_, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("MHT has no boundary")
	}
	reader := multipart.NewReader(message.Body, boundary)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, err
	}
	changed := false
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}
		media, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if media == "text/html" {
			encoding := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))
			var decoded io.Reader = bytes.NewReader(raw)
			switch encoding {
			case "quoted-printable":
				decoded = quotedprintable.NewReader(decoded)
			case "base64":
				decoded = base64.NewDecoder(base64.StdEncoding, decoded)
			}
			doc, err := io.ReadAll(decoded)
			if err != nil {
				return nil, err
			}
			updated := edit(doc)
			if !bytes.Equal(doc, updated) {
				changed = true
				var encoded bytes.Buffer
				qw := quotedprintable.NewWriter(&encoded)
				if _, err := qw.Write(updated); err != nil {
					return nil, err
				}
				if err := qw.Close(); err != nil {
					return nil, err
				}
				raw = encoded.Bytes()
				part.Header.Set("Content-Transfer-Encoding", "quoted-printable")
			}
		}
		w, err := writer.CreatePart(part.Header)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write(raw); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if !changed {
		return data, nil
	}
	headEnd := bytes.Index(data, []byte("\r\n\r\n"))
	sepLen := 4
	if headEnd < 0 {
		headEnd = bytes.Index(data, []byte("\n\n"))
		sepLen = 2
	}
	if headEnd < 0 {
		return nil, fmt.Errorf("MHT has no header separator")
	}
	out := append([]byte(nil), data[:headEnd+sepLen]...)
	return append(out, body.Bytes()...), nil
}
