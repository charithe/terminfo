package terminfo_test

import (
	"testing"

	"github.com/nhooyr/terminfo"
	"github.com/nhooyr/terminfo/caps"
)

func TestLoadTerminfo(t *testing.T) {
	ti, err := terminfo.GetTerminfo()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ti.Names[0])
	t.Log(ti.BoolCaps[caps.BackColorErase])
	t.Log(ti.NumericCaps[caps.MaxColors])
	t.Log(ti.StringCaps[caps.SetAForeground])
}
