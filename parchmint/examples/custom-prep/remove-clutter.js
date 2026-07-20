// Custom prep step (runs before parch serializes the page): strip obvious
// clutter so it's not baked into the archive. Returns a small stat object.
(() => {
	const junk = '.ad, .ads, .advert, .newsletter, .cookie, .cookie-banner, ' +
		'[id*="banner" i], [class*="popup" i], [aria-label*="advert" i]';
	let removed = 0;
	document.querySelectorAll(junk).forEach((el) => { el.remove(); removed++; });
	return { removed };
})
