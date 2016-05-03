package terminfo

import (
	"bytes"
	"testing"

	"github.com/nhooyr/terminfo/caps"
)

// TODO look at unibillium tests
func TestOpen(t *testing.T) {
	ti, err := LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q", ti.ExtStrings["kUP7"])
	t.Logf("%q", ti.Strings[caps.FlashScreen])
	b := bytes.NewBuffer(nil)
	ti.Strings[caps.PadChar] = "*"
	ti.Puts(b, ti.Strings[caps.FlashScreen], 1, 9600)
	t.Logf("%q", b.Bytes())
}

var result interface{}

func BenchmarkOpen(b *testing.B) {
	var r *Terminfo
	var err error
	for i := 0; i < b.N; i++ {
		r, err = LoadEnv()
		if err != nil {
			b.Fatal(err)
		}
	}
	result = r
}

func BenchmarkTiParm(b *testing.B) {
	ti, err := LoadEnv()
	if err != nil {
		b.Fatal(err)
	}
	var r string
	for i := 0; i < b.N; i++ {
		r = ti.Parm(caps.SetAForeground, 7, 5)
	}
	result = r
}

func BenchmarkParm(b *testing.B) {
	var r string
	for i := 0; i < b.N; i++ {
		r = Parm("%p1%:-10o %p1%:+10x %p1% 5X %p1%:-3.3d", 254)
	}
	result = r
}
