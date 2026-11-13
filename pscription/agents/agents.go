package agents

const (
	GoogleBot   = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	BingBot     = "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)"
	DuckDuckBot = "DuckDuckBot/1.1; (+http://duckduckgo.com/duckduckbot.html)"
	YandexBot   = "Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)"
	FacebookBot = "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)"
	ArchiveBot  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.110 Safari/537.36 archive.org_bot"
	CustomBot   = "Mozilla/5.0 (compatible; MyCustomBot/1.0; +https://example.com/bot)"
	// Safari is what every capture used to announce — from Chrome. A site
	// that branches on the user agent then serves its Safari paths (HLS
	// video, -webkit- CSS) to a browser that is not Safari. Kept for anyone
	// who needs the old behavior: pass it as the user agent explicitly.
	Safari = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15"

	// Current is a desktop Chrome user agent in the reduced form Chrome
	// itself sends (the minor version is frozen at 0.0.0 by design). It is
	// the FALLBACK and what the server-side fetchers send; a browser session
	// announces the running browser's own version instead — see
	// runner.Config.UserAgent.
	Current = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
)
