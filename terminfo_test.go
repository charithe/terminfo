package terminfo_test

import (
	"testing"

	"github.com/nhooyr/terminfo"
)

func TestLoadTerminfo(t *testing.T) {
	ti, err := terminfo.GetTermInfo()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ti.StringCaps[terminfo.SetAForeground])
}
