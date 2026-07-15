((id, css) => {
    let style = document.getElementById(id)
    if (style) { style.remove(); }
    style = document.createElement("style");
    style.id = id;
    style.innerHTML = css;
    document.head.appendChild(style); // Append the style element to the head
})