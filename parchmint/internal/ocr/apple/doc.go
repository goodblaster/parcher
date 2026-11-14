// Package apple is OCR through the macOS Vision framework: fast, accurate,
// and nothing to install.
//
// It exists only in a macOS build with cgo. Everything but this file carries
// `//go:build darwin && cgo` — including apple_vision.m, which Go would
// otherwise hand to every build and fail anything without cgo ("Objective-C
// source files not allowed"). In any other build the package is empty and
// "apple" is simply not a registered engine.
package apple

// Name is the engine's name on the command line.
const Name = "apple"
