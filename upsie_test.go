package megafile

import (
	"testing"

	"github.com/xyproto/vt"
)

func TestUpsie(t *testing.T) {
	const fullKernelVersion = false
	s, err := UpsieString(fullKernelVersion)
	o := vt.New()
	if err != nil {
		o.Err(err.Error())
		t.Fail()
	}
	o.Println(s)
}
