package logring

import (
	"fmt"
	"slices"
	"testing"
)

func TestRingKeepsLastLines(t *testing.T) {
	r := New(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(r, "line %d\n", i)
	}
	r.Write([]byte("partial"))
	if got := r.Lines(); !slices.Equal(got, []string{"line 3", "line 4", "line 5"}) {
		t.Fatalf("lines = %q", got)
	}
	r.Write([]byte(" done\r\nnext\n"))
	if got := r.Lines(); !slices.Equal(got, []string{"line 5", "partial done", "next"}) {
		t.Fatalf("lines = %q", got)
	}
}
