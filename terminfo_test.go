package terminfo_test

import (
	"testing"

	"github.com/nhooyr/terminfo"
	"github.com/nhooyr/terminfo/caps"
)

func TestOpen(t *testing.T) {
	ti, err := terminfo.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ti.Names[0])
	t.Log(ti.BoolCaps[caps.BackColorErase])
	t.Log(ti.NumericCaps[caps.MaxColors])
	t.Logf("%q", ti.StringCaps[caps.SetAForeground])
}

var result interface{}

func BenchmarkOpen(b *testing.B) {
	var r *terminfo.Terminfo
	for i := 0; i < b.N; i++ {
		r, _ = terminfo.Open()
	}
	result = r
}
