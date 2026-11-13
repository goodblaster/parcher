//go:build darwin && cgo

package apple

type Engine struct{}

// Available reports whether this build can use Apple Vision.
const Available = true

func (engine *Engine) String() string {
	return "apple"
}
