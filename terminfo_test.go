package terminfo_test

import (
	"testing"

	"github.com/nhooyr/terminfo"
	"github.com/nhooyr/terminfo/caps"
)

func TestOpen(t *testing.T) {
	ti, err := terminfo.OpenEnv()
	if err != nil {
		t.Fatal(err)
	}
	s, err := ti.Parm(caps.SetAForeground, 232)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q", s)
}

func TestParm(t *testing.T) {
	t.Logf("%q", terminfo.Parm("%p1%:-9d %p2%d", 343, 4343))
}

var result interface{}

func BenchmarkOpen(b *testing.B) {
	var r *terminfo.Terminfo
	var err error
	for i := 0; i < b.N; i++ {
		r, err = terminfo.OpenEnv()
		if err != nil {
			b.Fatal(err)
		}
	}
	result = r
}

func BenchmarkParm(b *testing.B) {
	ti, err := terminfo.OpenEnv()
	if err != nil {
		b.Fatal(err)
	}
	var r string
	v := ti.Strings[caps.SetAForeground]
	for i := 0; i < b.N; i++ {
		r = terminfo.Parm(v, 7, 5)
	}
	result = r
}
