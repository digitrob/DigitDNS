package logbuffer

import (
	"strings"
	"testing"
)

func TestBufferKeepsNewestLinesAndBounds(t *testing.T) {
	b := New(3, 1<<20)
	for _, line := range []string{"one\n", "two\n", "three\n", "four\n"} {
		if _, err := b.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	got := b.Lines(0)
	if len(got) != 3 || got[0] != "two" || got[2] != "four" {
		t.Fatalf("lines = %v", got)
	}
	if got := b.Lines(2); len(got) != 2 || got[0] != "three" || got[1] != "four" {
		t.Fatalf("limited lines = %v", got)
	}
}

func TestBufferJoinsSplitWrites(t *testing.T) {
	b := New(10, 1<<20)
	_, _ = b.Write([]byte("partial"))
	_, _ = b.Write([]byte(" line\nnext\n"))
	got := b.Lines(0)
	if len(got) != 2 || got[0] != "partial line" || got[1] != "next" {
		t.Fatalf("lines = %v", got)
	}
	if !strings.Contains(b.String(), "partial line") {
		t.Fatal("String did not include the joined line")
	}
}
