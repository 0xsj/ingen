package ambertransport

import (
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func FuzzDecodeValue(f *testing.F) {
	root, err := amber.Start()
	if err != nil {
		f.Fatal(err)
	}
	seed, err := EncodeValue(root)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed, false)
	f.Add("", true)
	f.Add("not-base64", false)
	f.Add("{}", true)

	f.Fuzz(func(t *testing.T, value string, ignore bool) {
		policy := amber.IncomingReject
		if ignore {
			policy = amber.IncomingIgnore
		}

		provenance, present, err := DecodeValue(value, policy)
		if err != nil {
			if present {
				t.Fatal("transport decoding returned both an error and a present value")
			}
			return
		}
		if !present {
			return
		}
		if err := provenance.Validate(); err != nil {
			t.Fatalf("accepted transport provenance does not validate: %v", err)
		}
	})
}
