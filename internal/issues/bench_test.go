package issues

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/outbound"
)

// BenchmarkIssueDelivery measures building, signing and handing over one
// recipient's copy of an issue: the per-reader cost of a send.
func BenchmarkIssueDelivery(b *testing.B) {
	e := setup(&testing.T{})
	ctx := context.Background()
	const members = 2000
	for i := 0; i < members; i++ {
		e.s.Lists.Add(ctx, e.l, fmt.Sprintf("r%d@x.org", i), "", true, lists.Signup{})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
		e.s.Schedule(ctx, w.ID, time.Time{})
		e.s.Tick(ctx)
		qctx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() { e.s.Queue.Run(qctx); close(done) }()
		for {
			if st, _ := e.s.Stats(ctx, w.ID); st.Queued == 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
		<-done
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*members), "µs/recipient")
	_ = outbound.KindIssue
}
