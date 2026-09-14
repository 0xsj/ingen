package amber

import (
	"encoding/json"
	"testing"
)

func FuzzFromJSON(f *testing.F) {
	root, err := Start()
	if err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(root)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed)
	f.Add([]byte{})
	f.Add([]byte(`null`))
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte(`{"version":1,"work_id":"not-an-id"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		provenance, err := FromJSON(data)
		if err != nil {
			return
		}

		if err := provenance.Validate(); err != nil {
			t.Fatalf("accepted provenance does not validate: %v", err)
		}

		encoded, err := json.Marshal(provenance)
		if err != nil {
			t.Fatalf("accepted provenance does not marshal: %v", err)
		}
		decoded, err := FromJSON(encoded)
		if err != nil {
			t.Fatalf("marshaled provenance does not round trip: %v", err)
		}
		if decoded.ExecutionID() != provenance.ExecutionID() {
			t.Fatalf("round trip changed execution ID: got %q want %q", decoded.ExecutionID(), provenance.ExecutionID())
		}
	})
}

func FuzzInspectIncomingJSON(f *testing.F) {
	root, err := Start()
	if err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(root)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(seed, false)
	f.Add([]byte{}, true)
	f.Add([]byte(`not-json`), false)
	f.Add([]byte(`{"version":1}`), true)

	f.Fuzz(func(t *testing.T, data []byte, ignore bool) {
		policy := IncomingReject
		if ignore {
			policy = IncomingIgnore
		}

		provenance, present, err := InspectIncomingJSON(data, policy)
		if err != nil {
			if present {
				t.Fatal("incoming inspection returned both an error and a present value")
			}
			return
		}
		if !present {
			return
		}
		if err := provenance.Validate(); err != nil {
			t.Fatalf("accepted incoming provenance does not validate: %v", err)
		}
	})
}
