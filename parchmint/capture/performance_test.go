package capture

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/actions"
	"github.com/goodblaster/parcher/pscription/runner"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func TestProbeTargetedCanvasWarming(t *testing.T) {
	needBrowser(t)
	source := `<!doctype html><body style="height:30000px"><canvas id="painted" width="20" height="20"></canvas><div style="position:absolute;top:25000px"><canvas id="lazy" width="20" height="20"></canvas></div><script>
 painted.getContext('2d').fillRect(0,0,20,20);
 new IntersectionObserver(es=>{for(const e of es)if(e.isIntersecting)e.target.getContext('2d').fillRect(0,0,20,20)}).observe(lazy);
 </script>`
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	session, err := runner.Start(ctx, archiveFixture(t, []byte(source)), runner.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var stats struct{ Bands, Warmed, StillBlank int }
	if err := session.Run(func(ctx context.Context) error { return js.WarmCanvases.Action(&stats).Do(ctx) }); err != nil {
		t.Fatal(err)
	}
	if stats.Bands != 1 || stats.Warmed != 1 || stats.StillBlank != 0 {
		t.Fatalf("unexpected warm pass: %+v", stats)
	}
	stats.Bands = 0
	stats.Warmed = 0
	if err := session.Run(func(ctx context.Context) error { return js.WarmCanvases.Action(&stats).Do(ctx) }); err != nil {
		t.Fatal(err)
	}
	if stats.Bands != 0 {
		t.Fatalf("painted canvases required %d bands", stats.Bands)
	}
}

func TestProbeParallelStylesheets(t *testing.T) {
	needBrowser(t)
	var active, peak atomic.Int32
	css := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p; p = peak.Load() {
			if peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprintf(w, "p {z-index: %s}", r.URL.Path[1:])
	}))
	defer css.Close()
	source := "<!doctype html><head>"
	for i := 0; i < 4; i++ {
		source += fmt.Sprintf(`<link rel="stylesheet" href="%s/%d">`, css.URL, i)
	}
	source += "</head><body><p>Order matters</p>"
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	session, err := runner.Start(ctx, archiveFixture(t, []byte(source)), runner.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	peak.Store(0)
	if err := session.Run(actions.InlineCrossOriginStylesheets()); err != nil {
		t.Fatal(err)
	}
	if peak.Load() < 2 {
		t.Fatal("stylesheets downloaded sequentially")
	}
	var order string
	if err := session.Run(func(ctx context.Context) error {
		return act.Evaluate(`getComputedStyle(document.querySelector('p')).zIndex`, &order).Do(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	if order != "3" {
		t.Fatalf("cascade changed: %s", order)
	}
}

func TestProbeLayoutHistory(t *testing.T) {
	needBrowser(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cfg := runner.DefaultConfig()
	cfg.PreNavigate = []act.Action{actions.NewSettleMonitor().Attach()}
	session, err := runner.Start(ctx, archiveFixture(t, []byte(`<!doctype html><body><p>Stable page</p>`)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var stats struct {
		Elapsed           int
		Stable, Monitored bool
	}
	wait := func(ctx context.Context) error { return js.WaitForStableLayout.Action(&stats, 300, 3000).Do(ctx) }
	if err := session.Run(wait); err != nil {
		t.Fatal(err)
	}
	if !stats.Monitored || !stats.Stable || stats.Elapsed >= 300 {
		t.Fatalf("stable page paid a fresh window: %+v", stats)
	}
	if err := session.Run(func(ctx context.Context) error {
		return act.Evaluate(`document.body.style.height='3000px'`, nil).Do(ctx)
	}, wait); err != nil {
		t.Fatal(err)
	}
	if !stats.Stable || stats.Elapsed < 300 {
		t.Fatalf("layout shift was not awaited: %+v", stats)
	}
}
