package federation

import (
	"testing"

	"github.com/james-gibson/isotope"
)

func TestSameCluster(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		self InstanceRecord
		rec  InstanceRecord
		want bool
	}{
		{"matching non-empty identity", InstanceRecord{ClusterID: "base-5100"}, InstanceRecord{ClusterID: "base-5100"}, true},
		{"differing identity", InstanceRecord{ClusterID: "base-5100"}, InstanceRecord{ClusterID: "base-5200"}, false},
		{"empty self identity never matches", InstanceRecord{}, InstanceRecord{}, false},
		{"empty peer identity never matches", InstanceRecord{ClusterID: "base-5100"}, InstanceRecord{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := SameCluster(tc.self, tc.rec); got != tc.want {
				t.Fatalf("SameCluster() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsotopeForMember(t *testing.T) {
	t.Parallel()

	rec, ok := IsotopeForMember(InstanceRecord{
		ID:       "alarm-5101",
		Role:     RoleIntroducer,
		Hostname: "127.0.0.1",
		Port:     5101,
	})
	if !ok {
		t.Fatal("IsotopeForMember() rejected a populated record")
	}
	if rec.Name != "alarm-5101" {
		t.Fatalf("Name = %q, want alarm-5101", rec.Name)
	}
	if rec.Endpoint != "http://127.0.0.1:5101" {
		t.Fatalf("Endpoint = %q, want http://127.0.0.1:5101", rec.Endpoint)
	}
	if rec.TrustRung != MeshMemberRung {
		t.Fatalf("TrustRung = %d, want %d", rec.TrustRung, MeshMemberRung)
	}
	if rec.RungName == "" {
		t.Fatal("RungName is empty; MeshMemberRung is not a canonical rung")
	}
	if rec.RegisteredAt.IsZero() {
		t.Fatal("RegisteredAt was not populated")
	}

	if _, found := IsotopeForMember(InstanceRecord{}); found {
		t.Fatal("IsotopeForMember() accepted an empty record")
	}

	// An identity without addressing info is still a valid isotope.
	rec, ok = IsotopeForMember(InstanceRecord{ID: "alarm-bare"})
	if !ok {
		t.Fatal("IsotopeForMember() rejected an identity without addressing")
	}
	if rec.Endpoint != "" {
		t.Fatalf("Endpoint = %q, want empty", rec.Endpoint)
	}
}

// fakeIsotopeSink records isotope sink calls for assertions.
type fakeIsotopeSink struct {
	upserts []string
	removed []string
}

func (f *fakeIsotopeSink) UpsertIsotope(rec isotope.Record) { f.upserts = append(f.upserts, rec.Name) }
func (f *fakeIsotopeSink) RemoveIsotope(name string)        { f.removed = append(f.removed, name) }

type mirrorCase struct {
	name       string
	self       InstanceRecord
	ev         Event
	wantUpsert int
	wantRemove int
}

func TestMirrorMember(t *testing.T) {
	t.Parallel()

	cases := []mirrorCase{
		{
			name:       "same cluster peer is mirrored",
			self:       InstanceRecord{ClusterID: "base-5100"},
			ev:         Event{Type: EventAdded, Record: InstanceRecord{ID: "peer", ClusterID: "base-5100"}},
			wantUpsert: 1,
		},
		{
			name:       "foreign cluster peer is ignored",
			self:       InstanceRecord{ClusterID: "base-5100"},
			ev:         Event{Type: EventAdded, Record: InstanceRecord{ID: "peer", ClusterID: "base-5200"}},
			wantUpsert: 0,
		},
		{
			name:       "no cluster identity means no mirroring",
			self:       InstanceRecord{},
			ev:         Event{Type: EventAdded, Record: InstanceRecord{ID: "peer"}},
			wantUpsert: 0,
		},
		{
			name:       "removed peer drops its isotope",
			self:       InstanceRecord{ClusterID: "base-5100"},
			ev:         Event{Type: EventRemoved, Record: InstanceRecord{ID: "peer", ClusterID: "base-5100"}},
			wantRemove: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := &fakeIsotopeSink{}
			MirrorMember(sink, tc.self, tc.ev)

			if len(sink.upserts) != tc.wantUpsert {
				t.Fatalf("upserts = %d, want %d", len(sink.upserts), tc.wantUpsert)
			}
			if len(sink.removed) != tc.wantRemove {
				t.Fatalf("removes = %d, want %d", len(sink.removed), tc.wantRemove)
			}
		})
	}
}
