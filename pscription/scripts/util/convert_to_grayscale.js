// Convert all images on the page to grayscale jpegs.

(() => {
    const images = document.querySelectorAll('img');
    images.forEach(img => {
        if (img.complete && img.naturalWidth > 0) {
            const canvas = document.createElement('canvas');
            canvas.width = img.naturalWidth;
            canvas.height = img.naturalHeight;
            const ctx = canvas.getContext('2d');
            ctx.drawImage(img, 0, 0);

            // Get the image data from the canvas
            const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
            const data = imageData.data;

            // Convert image to grayscale using luminance formula
            for (let i = 0; i < data.length; i += 4) {
                const r = data[i];
                const g = data[i + 1];
                const b = data[i + 2];
                const gray = 0.299 * r + 0.587 * g + 0.114 * b;
                data[i] = data[i + 1] = data[i + 2] = gray; // Set R, G, B to luminance value
            }

            // Put the modified image data back into the canvas
            ctx.putImageData(imageData, 0, 0);

            // Replace the original image with the base64 version
            img.src = canvas.toDataURL('image/jpeg', 0.90);
        }
    });
})