package terminfo_test

import (
	"log"
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
	log.Printf("%q\n", ti.ExtStrings)
	log.Printf("%d\n", len(ti.ExtStrings))
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
	// TODO GLOBAL
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
