package main

import "testing"

func TestParsePaper(t *testing.T) {
	cases := []struct {
		in      string
		w, h    float64
		wantErr bool
	}{
		{in: "", w: 0, h: 0},              // auto: sheet follows the layout
		{in: "auto", w: 0, h: 0},          //
		{in: "  AUTO ", w: 0, h: 0},       // case and space insensitive
		{in: "letter", w: 8.5, h: 11},     // named sheets carry a height
		{in: "A4", w: 8.27, h: 11.69},     //
		{in: "tabloid", w: 11, h: 17},     //
		{in: "10.5", w: 10.5, h: 0},       // bare number = width, tall page
		{in: "8.5x11", w: 8.5, h: 11},     // explicit sheet
		{in: " 8.5 x 11 ", w: 8.5, h: 11}, //
		{in: "0", wantErr: true},          // a zero sheet is not a sheet
		{in: "-3", wantErr: true},         //
		{in: "8.5x0", wantErr: true},      //
		{in: "foolscap", wantErr: true},   // unknown name
		{in: "8.5xwide", wantErr: true},   //
	}
	for _, c := range cases {
		w, h, err := parsePaper(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePaper(%q) = %v,%v, want error", c.in, w, h)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePaper(%q) unexpected error: %v", c.in, err)
			continue
		}
		if w != c.w || h != c.h {
			t.Errorf("parsePaper(%q) = %v,%v, want %v,%v", c.in, w, h, c.w, c.h)
		}
	}
}
