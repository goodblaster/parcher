// A CUSTOM user script. rx runs it in the page and captures whatever it
// returns. The pscription's `vars` object is passed as the first argument
// (unused here). This one collects the page's metadata as JSON.
//
// Any script of this shape works: `(vars) => value` (or an async version).
// Return anything JSON-serializable and it lands in the report's outputs.
((vars) => {
	const attr = (sel, name) => {
		const el = document.querySelector(sel);
		return el ? el.getAttribute(name) : null;
	};
	return {
		title: document.title,
		description: attr('meta[name="description"]', 'content'),
		canonical: attr('link[rel="canonical"]', 'href'),
		ogTitle: attr('meta[property="og:title"]', 'content'),
		ogImage: attr('meta[property="og:image"]', 'content'),
		lang: document.documentElement.lang || null,
		counts: {
			links: document.querySelectorAll('a[href]').length,
			images: document.querySelectorAll('img').length,
			headings: document.querySelectorAll('h1,h2,h3,h4,h5,h6').length,
		},
	};
})
