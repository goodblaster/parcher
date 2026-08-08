package main

import (
	"fmt"
	"strconv"
	"strings"
)

// paperSizes are the named sheets -paper accepts, in inches. A named size
// carries a HEIGHT as well as a width, so `-paper letter` produces real
// letter pages rather than an 8.5in-wide, 25-foot-long ribbon.
var paperSizes = map[string][2]float64{
	"letter":  {8.5, 11},
	"legal":   {8.5, 14},
	"tabloid": {11, 17},
	"a4":      {8.27, 11.69},
	"a3":      {11.69, 16.54},
	"a5":      {5.83, 8.27},
}

// parsePaper turns a -paper value into sheet dimensions in inches.
//
//	""/"auto"   → 0, 0    derive the sheet from the layout width, one tall
//	                      page (the historical behavior)
//	"letter"    → 8.5, 11 a named sheet, with real pagination
//	"10.5"      → 10.5, 0 an explicit WIDTH only, keeping the tall page
//	"8.5x11"    → 8.5, 11 explicit sheet
//
// Paper size never changes the LAYOUT width (-width does that); the page is
// rendered as laid out and scaled onto the sheet.
func parsePaper(s string) (widthIn, heightIn float64, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "auto" {
		return 0, 0, nil
	}
	if d, ok := paperSizes[s]; ok {
		return d[0], d[1], nil
	}
	if w, h, ok := strings.Cut(s, "x"); ok {
		widthIn, err = positiveInches(w)
		if err != nil {
			return 0, 0, err
		}
		heightIn, err = positiveInches(h)
		if err != nil {
			return 0, 0, err
		}
		return widthIn, heightIn, nil
	}
	widthIn, err = positiveInches(s)
	if err != nil {
		return 0, 0, fmt.Errorf("unknown paper size %q (want auto, %s, inches like 10.5, or WxH like 8.5x11)",
			s, strings.Join(paperNames(), ", "))
	}
	return widthIn, 0, nil
}

func positiveInches(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("bad paper dimension %q (want inches, e.g. 8.5)", s)
	}
	return v, nil
}

func paperNames() []string {
	// Stable order for help text and error messages.
	return []string{"letter", "legal", "tabloid", "a3", "a4", "a5"}
}
