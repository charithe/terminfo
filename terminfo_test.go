package terminfo_test

import (
	"os"
	"testing"

	"github.com/gdamore/tcell"
	"github.com/nhooyr/terminfo"
	"github.com/nhooyr/terminfo/caps"
)

func TestOpen(t *testing.T) {
	ti, err := terminfo.OpenEnv()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ti.Names[0])
	t.Log(ti.BoolCaps[caps.BackColorErase])
	t.Log(ti.NumericCaps[caps.MaxColors])
	t.Logf("%q", ti.StringCaps[caps.SetAForeground])
	t.Logf("%q", ti.Color(15, -1))
}

var result interface{}

func BenchmarkOpen(b *testing.B) {
	var r *terminfo.Terminfo
	for i := 0; i < b.N; i++ {
		r, _ = terminfo.OpenEnv()
	}
	result = r
}

func BenchmarkTcellOpen(b *testing.B) {
	var r *tcell.Terminfo
	for i := 0; i < b.N; i++ {
		r, _ = tcell.LookupTerminfo(os.Getenv("TERM"))
	}
	result = r
}

func BenchmarkParm(b *testing.B) {
	ti, err := terminfo.OpenEnv()
	if err != nil {
		b.Fatal(err)
	}
	var r string
	v := ti.StringCaps[caps.SetAForeground]
	for i := 0; i < b.N; i++ {
		r = ti.Parm(v, 7, 5)
	}
	result = r
}

func BenchmarkTcellParm(b *testing.B) {
	ti, err := tcell.LookupTerminfo(os.Getenv("TERM"))
	if err != nil {
		b.Fatal(err)
	}
	var r string
	for i := 0; i < b.N; i++ {
		r = ti.TParm(ti.SetFg, 7, 5)
	}
	result = r
}
