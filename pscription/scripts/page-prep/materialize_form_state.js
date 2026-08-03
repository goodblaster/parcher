// Writes runtime-only form state back into the DOM as attributes.
//
// A serializer that walks MARKUP (Chrome's MHTML writer) sees the attribute,
// not the IDL property, so a typed value, a ticked box and a chosen <option>
// all serialize as if the user had never touched them. Measured on a probe
// page: value, checked and selectedIndex were all lost, while <details open>
// survived — because that one IS an attribute.
//
// Passwords are deliberately never baked in: an archive is a file that gets
// copied around, and a typed password is not something to write into it.
(() => {
    let inputs = 0;
    let textareas = 0;
    let selects = 0;

    for (const el of document.querySelectorAll('input')) {
        const type = (el.type || 'text').toLowerCase();

        if (type === 'password') continue;

        if (type === 'checkbox' || type === 'radio') {
            if (el.checked === el.hasAttribute('checked')) continue;
            if (el.checked) el.setAttribute('checked', '');
            else el.removeAttribute('checked');
            inputs++;
            continue;
        }

        // A file input's value cannot be set, and the button types carry a
        // label in value rather than user state.
        if (['file', 'image', 'submit', 'reset', 'button'].includes(type)) continue;

        if (el.value !== el.getAttribute('value')) {
            el.setAttribute('value', el.value);
            inputs++;
        }
    }

    // A textarea's default value is its TEXT CONTENT, not an attribute.
    for (const el of document.querySelectorAll('textarea')) {
        if (el.value !== el.textContent) {
            el.textContent = el.value;
            textareas++;
        }
    }

    for (const el of document.querySelectorAll('select')) {
        let changed = false;
        for (const option of el.options) {
            if (option.selected === option.hasAttribute('selected')) continue;
            if (option.selected) option.setAttribute('selected', '');
            else option.removeAttribute('selected');
            changed = true;
        }
        if (changed) selects++;
    }

    return { inputs, textareas, selects };
})
