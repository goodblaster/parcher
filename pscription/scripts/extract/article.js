// Extracts the main article content of a page as clean text/markdown-ish
// output, discarding nav, sidebars, ads, and boilerplate. A dependency-free
// readability heuristic: score block containers by the density of their
// paragraph text, take the winner, and serialize its meaningful descendants.
//
// Returns { title, byline, url, text, wordCount, html } where `text` is the
// article as plain paragraphs/headings/lists (good for feeding an LLM) and
// `html` is the cleaned inner HTML of the chosen container.
(() => {
    const clone = document.cloneNode(true);
    const doc = clone.ownerDocument || clone;

    // Strip elements that never carry article content.
    const junk = 'script, style, noscript, nav, header, footer, aside, form, ' +
        'iframe, svg, [role="navigation"], [role="banner"], [role="complementary"], ' +
        '[aria-hidden="true"], .nav, .menu, .sidebar, .ad, .ads, .advert, ' +
        '.promo, .social, .share, .comment, .comments, .related, .newsletter, .cookie';
    doc.querySelectorAll(junk).forEach((el) => el.remove());

    const textLen = (el) => (el.textContent || '').replace(/\s+/g, ' ').trim().length;

    // Prefer an explicit <article>/main-role container if it has real text;
    // otherwise score candidates by paragraph text density.
    let best = null;
    let bestScore = 0;
    const explicit = doc.querySelector('article, [role="main"], main');
    if (explicit && textLen(explicit) > 200) {
        best = explicit;
    } else {
        const candidates = doc.querySelectorAll('article, main, section, div');
        for (const el of candidates) {
            const paras = el.querySelectorAll(':scope > p, :scope > div > p');
            let score = 0;
            for (const p of paras) {
                const len = textLen(p);
                if (len > 25) score += len;
            }
            // Penalize link-heavy blocks (nav/lists of links).
            const linkLen = Array.from(el.querySelectorAll('a'))
                .reduce((n, a) => n + textLen(a), 0);
            score -= linkLen * 0.5;
            if (score > bestScore) {
                bestScore = score;
                best = el;
            }
        }
    }

    if (!best) best = doc.body;

    // Serialize meaningful descendants to text in document order.
    const lines = [];
    const walk = (node) => {
        for (const child of node.children) {
            const tag = child.tagName.toLowerCase();
            const t = (child.textContent || '').replace(/\s+/g, ' ').trim();
            if (/^h[1-6]$/.test(tag)) {
                if (t) lines.push('#'.repeat(+tag[1]) + ' ' + t);
            } else if (tag === 'p' || tag === 'blockquote') {
                if (t) lines.push(t);
            } else if (tag === 'li') {
                if (t) lines.push('- ' + t);
            } else if (tag === 'pre') {
                if (t) lines.push('```\n' + child.textContent.trim() + '\n```');
            } else if (child.children.length) {
                walk(child);
            } else if (t && (tag === 'div' || tag === 'span')) {
                lines.push(t);
            }
        }
    };
    walk(best);

    const text = lines.join('\n\n').replace(/\n{3,}/g, '\n\n').trim();

    const titleEl = document.querySelector('h1') || document.querySelector('title');
    const byline = (document.querySelector('[rel="author"], .author, .byline') || {}).textContent;

    return {
        title: (titleEl ? titleEl.textContent : document.title).trim(),
        byline: byline ? byline.replace(/\s+/g, ' ').trim() : null,
        url: location.href,
        text,
        wordCount: text ? text.split(/\s+/).length : 0,
        html: best.innerHTML,
    };
})
