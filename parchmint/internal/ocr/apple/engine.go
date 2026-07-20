//go:build darwin && cgo

package apple

type Engine struct{}

func (engine *Engine) String() string {
	return "apple"
}
