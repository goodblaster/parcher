// Custom script used by headings.rx: collect the page's heading outline
// (h1–h3) as structured data.
(() => Array.from(document.querySelectorAll('h1, h2, h3'))
	.map((h) => ({
		level: Number(h.tagName[1]),
		text: h.textContent.replace(/\s+/g, ' ').trim(),
	}))
	.filter((h) => h.text.length > 0))
