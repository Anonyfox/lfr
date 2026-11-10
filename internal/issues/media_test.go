package issues

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/blob"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/store"
)

func TestMediaReplacementInvalidatesOnlyChangedTestCopies(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	blobs, err := blob.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.s.Blobs = blobs
	w, err := e.s.Create(ctx, "bakers", Draft{Markdown: "# News\n\n![Garden](images/garden.png)\n\n[Read more](https://example.org/read)"})
	if err != nil {
		t.Fatal(err)
	}
	pngData := func(c color.RGBA) []byte {
		im := image.NewRGBA(image.Rect(0, 0, 1, 1))
		im.Set(0, 0, c)
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	first := pngData(color.RGBA{R: 255, A: 255})
	second := pngData(color.RGBA{G: 255, A: 255})
	for _, tc := range []struct {
		name  string
		data  []byte
		reset bool
	}{
		{"first upload", first, true}, {"identical upload", first, false}, {"replacement", second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := e.s.Test(ctx, w.ID, []string{"owner@example.org"}); err != nil {
				t.Fatal(err)
			}
			if err := e.s.SetMedia(ctx, w.ID, []File{{Path: "images/garden.png", Data: tc.data}}); err != nil {
				t.Fatal(err)
			}
			got, err := e.s.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Tested.IsZero() != tc.reset {
				t.Fatalf("test-copy invalidation: %+v", got)
			}
			media, err := e.s.Media(ctx, w.ID)
			if err != nil || len(media) != 1 {
				t.Fatalf("media: %v %v", media, err)
			}
			m := media["images/garden.png"]
			b, typ, err := e.s.MediaFile(ctx, e.l.ID, m.Name())
			if err != nil || typ != "image/png" || !bytes.Equal(b, tc.data) {
				t.Fatalf("served media differs: %s %v", typ, err)
			}
			for _, bad := range []struct {
				list int64
				name string
			}{{e.l.ID, m.Hash + ".jpg"}, {e.l.ID + 1, m.Name()}, {e.l.ID, "missing.png"}} {
				if _, _, err := e.s.MediaFile(ctx, bad.list, bad.name); !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("served inaccessible media: %v", err)
				}
			}
		})
	}
	before, _ := e.s.Media(ctx, w.ID)
	if err := e.s.SetMedia(ctx, w.ID, []File{{Path: "bad.png", Data: []byte("not an image")}}); err == nil {
		t.Fatal("invalid image accepted")
	}
	after, _ := e.s.Media(ctx, w.ID)
	if after["images/garden.png"].Hash != before["images/garden.png"].Hash {
		t.Fatal("failed upload lost existing media")
	}
	e.r.Media = func(_ int64) map[string]Media { return after }
	web, err := e.r.WebHTML(e.l, w)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(web), after["images/garden.png"].Name()) || strings.Contains(string(web), "/click/") {
		t.Fatalf("archive media/tracking: %s", web)
	}
	target, ok := e.r.Link(e.l, w, 0)
	if !ok || target != "https://example.org/read" {
		t.Fatalf("click destination: %q %v", target, ok)
	}
	for _, i := range []int{-1, 1, 100} {
		if _, ok := e.r.Link(e.l, w, i); ok {
			t.Errorf("accepted link index %d", i)
		}
	}
	raw, err := e.r.Message(e.l, w, Recipient{Email: "reader@example.org"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p := msg.Parse(raw); !strings.Contains(p.HTML, after["images/garden.png"].Name()) {
		t.Fatal("email missing uploaded image")
	}
	if err := e.s.SetMedia(ctx, w.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.s.MediaFile(ctx, e.l.ID, after["images/garden.png"].Name()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("removed media still exposed: %v", err)
	}
}

func TestIssueArchiveOrderingAndRemoval(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, err := e.s.Create(ctx, "bakers", Draft{Markdown: "# First"})
	if err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(time.Hour)
	b, err := e.s.Create(ctx, "bakers", Draft{Markdown: "# Second"})
	if err != nil {
		t.Fatal(err)
	}
	all, err := e.s.List(ctx, e.l.ID, false, 0)
	if err != nil || len(all) != 2 || all[0].ID != b.ID {
		t.Fatalf("archive order: %+v %v", all, err)
	}
	published, err := e.s.List(ctx, e.l.ID, true, 1)
	if err != nil || len(published) != 0 {
		t.Fatalf("draft published: %+v %v", published, err)
	}
	if err := e.s.Remove(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Get(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("removed issue survives: %v", err)
	}
}
