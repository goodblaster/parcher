package std

import (
	"strings"
)

// Word represents a single word with its bounding box.
// Bounds represent percentage of page width and height.
type Word struct {
	Text   string  `json:"text"`
	Top    float64 `json:"top"`
	Left   float64 `json:"left"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Rect struct {
	Top    float64 `json:"top"`
	Left   float64 `json:"left"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Line represents a line of text containing multiple words
type Line struct {
	LineText   string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Rect       Rect    `json:"rect"`
	Words      []Word  `json:"words"`
}

// Text concatenates the text of all words in the line
func (l Line) Text() string {
	var parts []string
	for _, word := range l.Words {
		parts = append(parts, word.Text)
	}
	return strings.Join(parts, " ")
}

// Paragraph represents a paragraph containing multiple lines
type Paragraph struct {
	Lines []Line
}

// Text concatenates the text of all lines in the paragraph
func (p Paragraph) Text() string {
	var parts []string
	for _, line := range p.Lines {
		parts = append(parts, line.Text())
	}
	return strings.Join(parts, "\n")
}

// Page represents a page containing multiple paragraphs
type Page struct {
	Lines []Line `json:"lines"`
}

// Text concatenates the text of all paragraphs in the page
func (pg Page) Text() string {
	var parts []string
	for _, line := range pg.Lines {
		parts = append(parts, line.Text())
	}
	return strings.Join(parts, " ")
}
