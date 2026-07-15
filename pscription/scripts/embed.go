package scripts

import (
	"embed"
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
)

// Media handling scripts
const (
	CaptureVideoFrame ScriptFile = "media/capture_video_frame.js"
	ConvertSvgToPng   ScriptFile = "media/convert_svg_to_png.js"
	ConvertSvgToJpg   ScriptFile = "media/convert_svg_to_jpg.js"
	LocateSvgElements ScriptFile = "media/locate_svg_elements.js"
)

// Resource embedding scripts
const (
	ExtractBackgroundURLs  ScriptFile = "resources/extract_background_urls.js"
	InjectBackgroundImages ScriptFile = "resources/inject_background_images.js"
	ExtractImageURLs       ScriptFile = "resources/extract_image_urls.js"
	InjectImages           ScriptFile = "resources/inject_images.js"
	EmbedImages            ScriptFile = "resources/embed_images.js"
	EmbedBackgroundImages  ScriptFile = "resources/embed_background_images.js"
	RasterizeFontIcons     ScriptFile = "resources/rasterize_font_icons.js"
	InjectCss              ScriptFile = "resources/inject_css.js"
	RemoveUnusedCSS        ScriptFile = "resources/remove_unused_css.js"
	RemoveUnusedFonts      ScriptFile = "resources/remove_unused_fonts.js"
	InlineIframes          ScriptFile = "resources/inline_iframes.js"
)

// Utility scripts
const (
	DetectLanguage     ScriptFile = "util/detect_language.js"
	ExtractLinks       ScriptFile = "util/extract_links.js"
	ConvertToGrayscale ScriptFile = "util/convert_to_grayscale.js"
)
