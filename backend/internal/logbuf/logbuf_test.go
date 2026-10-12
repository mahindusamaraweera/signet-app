package logbuf

import (
	"reflect"
	"testing"
)

func TestRingKeepsNewestCompleteLines(t *testing.T) {
	r := New(3)
	r.Write([]byte("one\ntw"))
	r.Write([]byte("o\nthree\n"))
	if got := r.Lines(); !reflect.DeepEqual(got, []string{"one", "two", "three"}) {
		t.Fatalf("got %q", got)
	}
	r.Write([]byte("four\nfive\nunfinished"))
	if got := r.Lines(); !reflect.DeepEqual(got, []string{"three", "four", "five"}) {
		t.Fatalf("after wrap got %q", got)
	}
	r.Write([]byte(" line\n"))
	if got := r.Lines(); !reflect.DeepEqual(got, []string{"four", "five", "unfinished line"}) {
		t.Fatalf("partial line not joined: %q", got)
	}
}
