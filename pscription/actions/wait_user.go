package actions

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/chromedp/chromedp"
)

// WaitForUser pauses the pscription until the operator presses Enter. It
// prints prompt to stderr (stdout is reserved for the report) and blocks on
// stdin. Meant for headful runs where a human logs in, dismisses something
// by hand, or otherwise readies the page before the remaining steps run.
//
// If stdin isn't a terminal (e.g. the run is itself scripted), it returns
// immediately so a pscription with a wait step doesn't hang a pipeline.
func WaitForUser(prompt string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if prompt == "" {
			prompt = "Paused. Complete any manual steps in the browser, then press Enter to continue..."
		}
		if !stdinIsTerminal() {
			fmt.Fprintln(os.Stderr, "wait-for-user: stdin is not a terminal; continuing without pausing")
			return nil
		}
		fmt.Fprintln(os.Stderr, prompt)

		done := make(chan struct{})
		go func() {
			bufio.NewReader(os.Stdin).ReadString('\n')
			close(done)
		}()

		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
