package scripts

import (
	"embed"
	"sort"
)

//go:embed */*.js
var Files embed.FS

// Page preparation scripts
const (
	ScrollAndWaitImages    ScriptFile = "page-prep/scroll_and_wait_images.js"
	ForceImageLoading      ScriptFile = "page-prep/force_image_loading.js"
	PreseedConsent         ScriptFile = "page-prep/preseed_consent.js"
	DismissCMP             ScriptFile = "page-prep/dismiss_cmp.js"
	DismissPopups          ScriptFile = "page-prep/dismiss_popups.js"
	HideOverlays           ScriptFile = "page-prep/hide_overlays.js"
	RemoveBlockingOverlays ScriptFile = "page-prep/remove_blocking_overlays.js"
	NormalizeViewportUnits ScriptFile = "page-prep/normalize_viewport_units.js"
	RewriteLinks           ScriptFile = "page-prep/rewrite_links.js"
	WaitForImages          ScriptFile = "page-prep/wait_for_images.js"
	WaitForQuiet           ScriptFile = "page-prep/wait_for_quiet.js"
	SettleMonitor          ScriptFile = "page-prep/settle_monitor.js"
	WaitForStableLayout    ScriptFile = "page-prep/wait_for_stable_layout.js"
	WaitForSliders         ScriptFile = "page-prep/wait_for_sliders.js"
	DetectSliderDefect     ScriptFile = "page-prep/detect_slider_defect.js"
	PreserveWebGLBuffer    ScriptFile = "page-prep/preserve_webgl_buffer.js"

	// Serialization-visible state: runtime-only state that a serializer
	// walking markup cannot see, written back into the DOM.
	MaterializeFormState      ScriptFile = "page-prep/materialize_form_state.js"
	FlattenAdoptedStylesheets ScriptFile = "page-prep/flatten_adopted_stylesheets.js"
	LinkShadowStyles          ScriptFile = "page-prep/link_shadow_styles.js"
	PreserveHiddenAttr        ScriptFile = "page-prep/preserve_hidden_attr.js"
)

// Media handling scripts
const (
	CaptureVideoFrame      ScriptFile = "media/capture_video_frame.js"
	ConvertSvgToPng        ScriptFile = "media/convert_svg_to_png.js"
	ConvertSvgToJpg        ScriptFile = "media/convert_svg_to_jpg.js"
	LocateSvgElements      ScriptFile = "media/locate_svg_elements.js"
	WarmCanvases           ScriptFile = "media/warm_canvases.js"
	RasterizeCanvases      ScriptFile = "media/rasterize_canvases.js"
	ReplaceCanvasWithImage ScriptFile = "media/replace_canvas_with_image.js"
	BakeImageMarks         ScriptFile = "media/bake_image_marks.js"
	OverlayOcrText         ScriptFile = "media/overlay_ocr_text.js"
	RemoveJunkIframes      ScriptFile = "media/remove_junk_iframes.js"
	LocateOpaqueIframes    ScriptFile = "media/locate_opaque_iframes.js"
	FocusOpaqueIframe      ScriptFile = "media/focus_opaque_iframe.js"
	ReplaceIframeWithImage ScriptFile = "media/replace_iframe_with_image.js"
)

// Resource embedding scripts
const (
	ExtractBackgroundURLs  ScriptFile = "resources/extract_background_urls.js"
	InjectBackgroundImages ScriptFile = "resources/inject_background_images.js"
	ExtractFontFaces       ScriptFile = "resources/extract_font_faces.js"
	ExtractImageURLs       ScriptFile = "resources/extract_image_urls.js"
	InjectImages           ScriptFile = "resources/inject_images.js"
	EmbedImages            ScriptFile = "resources/embed_images.js"
	EmbedBackgroundImages  ScriptFile = "resources/embed_background_images.js"
	RasterizeFontIcons     ScriptFile = "resources/rasterize_font_icons.js"
	InjectCss              ScriptFile = "resources/inject_css.js"
	RemoveUnusedCSS        ScriptFile = "resources/remove_unused_css.js"
	RemoveUnusedFonts      ScriptFile = "resources/remove_unused_fonts.js"
	InlineIframes          ScriptFile = "resources/inline_iframes.js"
	ListCrossOriginSheets  ScriptFile = "resources/list_crossorigin_sheets.js"
	InlineSheetText        ScriptFile = "resources/inline_sheet_text.js"
)

// Utility scripts
const (
	DetectLanguage     ScriptFile = "util/detect_language.js"
	ExtractLinks       ScriptFile = "util/extract_links.js"
	ConvertToGrayscale ScriptFile = "util/convert_to_grayscale.js"
)

// Built-in extraction scripts, referenced in a pscription as "@name".
const (
	ExtractArticle   ScriptFile = "extract/article.js"
	ExtractPageLinks ScriptFile = "extract/links.js"
	ExtractText      ScriptFile = "extract/text.js"
	ExtractTextLayer ScriptFile = "extract/text_layer.js"
	ApplyTextMarks   ScriptFile = "extract/apply_text_marks.js"
	RevealHidden     ScriptFile = "extract/reveal_hidden.js"
)

// builtinScripts maps the "@name" reference to its embedded script.
var builtinScripts = map[string]ScriptFile{
	"article": ExtractArticle,
	"links":   ExtractPageLinks,
	"text":    ExtractText,
}

// BuiltinScript resolves a "@name" reference (without the @) to an embedded
// extraction script.
func BuiltinScript(name string) (ScriptFile, bool) {
	sf, ok := builtinScripts[name]
	return sf, ok
}

// BuiltinScriptNames lists the available "@name" scripts.
func BuiltinScriptNames() []string {
	names := make([]string, 0, len(builtinScripts))
	for name := range builtinScripts {
		names = append(names, name)
	}
	sort.Strings(names) // error messages list these; map order is random
	return names
}
