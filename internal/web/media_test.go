package web

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/blob"
	"github.com/Anonyfox/lfr/internal/issues"
)

func TestUploadedMediaAndAssetsStayWithinTheList(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	bs, err := blob.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.srv.Issues.Blobs = bs
	w, err := e.srv.Issues.Create(ctx, "bakers", issues.Draft{Markdown: "# Garden\n\n![A garden](images/garden.png)"})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 30))
	if err := e.srv.Issues.SetMedia(ctx, w.ID, []issues.File{{Path: "images/garden.png", Data: data}}); err != nil {
		t.Fatal(err)
	}
	media, err := e.srv.Issues.Media(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	name := media["images/garden.png"].Name()
	r := e.do("GET", "https://news.example.com/media/"+name, "")
	if r.Code != 200 || r.Header().Get("Content-Type") != "image/png" || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") || !bytes.Equal(r.Body.Bytes(), data) {
		t.Fatalf("media: %d %v", r.Code, r.Header())
	}
	for _, target := range []string{"https://mail.example.com/lists/plain/media/" + name, "https://news.example.com/media/missing.png"} {
		if r := e.do("GET", target, ""); r.Code != 404 {
			t.Fatalf("inaccessible media: %s %d", target, r.Code)
		}
	}
	if err := e.srv.Sites.Put(e.l.Slug, map[string][]byte{"assets/style.css": []byte("body{color:red}")}); err != nil {
		t.Fatal(err)
	}
	r = e.do("GET", "https://news.example.com/assets/style.css", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), "color:red") || r.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("asset: %d %s", r.Code, r.Body.String())
	}
	if r := e.do("GET", "https://news.example.com/assets/missing.css", ""); r.Code != 404 {
		t.Fatal("missing asset served")
	}
	for _, tc := range []struct {
		method, target string
		status         int
	}{
		{"GET", "https://mail.example.com/", 200}, {"PUT", "https://news.example.com/", 405}, {"GET", "https://news.example.com/archive", 200}, {"GET", "https://news.example.com/subscribe", 303}, {"OPTIONS", "https://news.example.com/subscribe", 204},
	} {
		r := e.do(tc.method, tc.target, "")
		if r.Code != tc.status {
			t.Errorf("%s %s: %d", tc.method, tc.target, r.Code)
		}
	}
}
