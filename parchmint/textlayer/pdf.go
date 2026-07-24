package textlayer

import (
	"bytes"
	"fmt"
	"io"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDF support: the layer travels as an embedded-file attachment
// (/EmbeddedFiles). PDF viewers ignore it; parch text/find pull it back
// out. The PDF also carries native selectable text (Chrome's printToPDF)
// plus, for OCR'd images, invisible positioned overlay text — but the
// attachment is what gives `find` the structured blocks and word boxes,
// so it stays at full parity with .html/.mht.
const PDFLayerName = "parchmint-text-layer.json"

// IsPDF reports whether the bytes are a PDF.
func IsPDF(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), []byte("%PDF-"))
}

func pdfConf() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	// printToPDF output is well-formed but occasionally trips strict
	// validation; relaxed keeps us robust without rewriting the PDF.
	conf.ValidationMode = model.ValidationRelaxed
	return conf
}

// EmbedInPDF attaches layerJSON to the PDF as PDFLayerName, replacing any
// prior copy so re-embedding is idempotent.
func EmbedInPDF(pdf, layerJSON []byte) ([]byte, error) {
	conf := pdfConf()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), conf)
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	// Drop an existing attachment so re-embedding is idempotent (ignore
	// "not found").
	_, _ = ctx.RemoveAttachment(model.Attachment{ID: PDFLayerName})

	if err := ctx.AddAttachment(model.Attachment{
		Reader:   bytes.NewReader(layerJSON),
		ID:       PDFLayerName,
		FileName: PDFLayerName,
		Desc:     Format,
	}, false); err != nil {
		return nil, fmt.Errorf("attach text layer: %w", err)
	}

	var buf bytes.Buffer
	if err := api.Write(ctx, &buf, conf); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// pdfLayerBytes extracts the embedded layer JSON from a PDF.
// ExtractAttachmentsRaw (not Attachments/ListAttachments, which return
// metadata with a nil Reader) gives the content in memory.
func pdfLayerBytes(pdf []byte) ([]byte, error) {
	aa, err := api.ExtractAttachmentsRaw(bytes.NewReader(pdf), "", []string{PDFLayerName}, pdfConf())
	if err != nil {
		return nil, fmt.Errorf("read pdf attachments: %w", err)
	}
	for _, a := range aa {
		if a.Reader != nil {
			return io.ReadAll(a.Reader)
		}
	}
	return nil, fmt.Errorf("no embedded text layer attachment")
}
