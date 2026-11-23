package capture

import (
	"bytes"
	"encoding/json"

	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/actions"
)

// Report is the versioned capture outcome used both in --report and in HTML/
// MHT archives. Output is omitted in the embedded copy; '-' denotes stdout.
type Report struct {
	Schema   string                `json:"schema"`
	URL      string                `json:"url"`
	FinalURL string                `json:"final_url"`
	Output   string                `json:"output,omitempty"`
	MIME     string                `json:"mime"`
	Access   *actions.AccessReport `json:"access"`
}

func (s *Snapshot) Report(output string) Report {
	return Report{Schema: "parch-capture/1", URL: s.URL, FinalURL: s.FinalURL,
		Output: output, MIME: s.MIME, Access: s.Access}
}

func embedCaptureReport(s *Snapshot) error {
	if s.Access == nil || (s.MIME != "text/html" && s.MIME != "multipart/related") {
		return nil
	}
	payload, err := json.Marshal(s.Report(""))
	if err != nil {
		return err
	}
	// json.Marshal escapes '<', including any hostile closing script tags.
	tag := append([]byte("\n<script type=\"application/x-parch-capture+json\">"), payload...)
	tag = append(tag, []byte("</script>\n")...)
	doc := s.Bytes
	if s.MIME == "multipart/related" {
		doc, err = textlayer.MHTDocument(s.Bytes)
		if err != nil {
			return err
		}
	}
	at := bytes.LastIndex(bytes.ToLower(doc), []byte("</body>"))
	if at < 0 {
		at = len(doc)
	}
	embedded := make([]byte, 0, len(doc)+len(tag))
	embedded = append(embedded, doc[:at]...)
	embedded = append(embedded, tag...)
	embedded = append(embedded, doc[at:]...)
	if s.MIME == "multipart/related" {
		s.Bytes, err = textlayer.ReplaceMHTDocument(s.Bytes, embedded)
	} else {
		s.Bytes = embedded
	}
	return err
}
