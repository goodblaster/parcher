// Runs on every new document BEFORE the page's own JavaScript (registered
// via Page.addScriptToEvaluateOnNewDocument). Forces preserveDrawingBuffer
// on every WebGL context the page creates, so the drawing buffer survives
// frame presentation and drawImage()/toDataURL() can read real pixels at
// capture time. Without this, reading a WebGL canvas after its frame is
// presented yields transparent black — WebGL scenes would archive as blank
// rectangles.
//
// Nearly all WebGL apps redraw the full frame every tick, so preserving
// the buffer doesn't change what renders; and this only affects the
// capture session, never real visitors.
(() => {
    const origGet = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (type, attrs) {
        if (type === 'webgl' || type === 'webgl2' || type === 'experimental-webgl') {
            attrs = Object.assign({}, attrs, { preserveDrawingBuffer: true });
            return origGet.call(this, type, attrs);
        }
        // Pass other context types through with the original arity so a
        // bare getContext('2d') stays a one-argument call.
        return arguments.length < 2 ? origGet.call(this, type) : origGet.call(this, type, attrs);
    };
})
