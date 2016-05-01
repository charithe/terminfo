package terminfo_test

import (
	"testing"

	"github.com/nhooyr/terminfo"
	"github.com/nhooyr/terminfo/caps"
)

func TestOpen(t *testing.T) {
	ti, err := terminfo.Load("xterm")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q", ti.ExtStrings["kUP7"])
}

var result interface{}

func BenchmarkOpen(b *testing.B) {
	var r *terminfo.Terminfo
	var err error
	for i := 0; i < b.N; i++ {
		r, err = terminfo.LoadEnv()
		if err != nil {
			b.Fatal(err)
		}
	}
	result = r
}

func BenchmarkTiParm(b *testing.B) {
	ti, err := terminfo.LoadEnv()
	if err != nil {
		b.Fatal(err)
	}
	var r string
	for i := 0; i < b.N; i++ {
		r = terminfo.Parm(ti.Strings[caps.SetAForeground], 7, 5)
	}
	result = r
}

// TODO somehow there are 6 allocations/op?
func BenchmarkParm(b *testing.B) {
	var r string
	for i := 0; i < b.N; i++ {
		r = terminfo.Parm("%p1%:-10o %p1%:+10x %p1% 5X %p1%:-3.3d", 254)
	}
	result = r
}
