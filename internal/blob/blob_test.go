package blob

import (
	"os"
	"testing"
)

func TestPutGetRemove(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h1, err := s.Put([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := s.Put([]byte("hello"))
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hashes %s %s", h1, h2)
	}
	got, err := s.Get(h1)
	if err != nil || string(got) != "hello" {
		t.Fatalf("get = %q, %v", got, err)
	}
	var n int
	s.Walk(func(string) error { n++; return nil })
	if n != 1 {
		t.Fatalf("walk saw %d blobs", n)
	}
	if err := s.Remove(h1); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(h1); err != nil {
		t.Fatal("second remove:", err)
	}
	if _, err := s.Get(h1); !os.IsNotExist(err) {
		t.Fatalf("get after remove: %v", err)
	}
	if _, err := s.Get("../../etc/passwd"); err == nil {
		t.Fatal("accepted a path as hash")
	}
}
