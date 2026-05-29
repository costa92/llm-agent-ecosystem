package contract

import (
	"errors"
	"testing"
)

func TestNormalizeRecordKind_DefaultsBlankToEpisodic(t *testing.T) {
	got, err := NormalizeRecordKind("   ")
	if err != nil {
		t.Fatalf("NormalizeRecordKind(blank): %v", err)
	}
	if got != RecordKindEpisodic {
		t.Fatalf("NormalizeRecordKind(blank) = %q, want %q", got, RecordKindEpisodic)
	}
}

func TestNormalizeRecordKind_AcceptsCanonicalKinds(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "working", want: RecordKindWorking},
		{in: "episodic", want: RecordKindEpisodic},
		{in: "semantic", want: RecordKindSemantic},
		{in: " Working ", want: RecordKindWorking},
	} {
		got, err := NormalizeRecordKind(tc.in)
		if err != nil {
			t.Fatalf("NormalizeRecordKind(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeRecordKind(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRecordKind_RejectsUnknownKind(t *testing.T) {
	_, err := NormalizeRecordKind("archive")
	if !errors.Is(err, ErrInvalidRecordKind) {
		t.Fatalf("NormalizeRecordKind(archive) err = %v, want ErrInvalidRecordKind", err)
	}
}

func TestMemoryRecordNormalizeWriteDefaults_DefaultsBlankKind(t *testing.T) {
	record, err := (MemoryRecord{}).NormalizeWriteDefaults()
	if err != nil {
		t.Fatalf("NormalizeWriteDefaults: %v", err)
	}
	if record.Kind != RecordKindEpisodic {
		t.Fatalf("NormalizeWriteDefaults kind = %q, want %q", record.Kind, RecordKindEpisodic)
	}
}

func TestMemoryRecordSetWorkingDefault_OnlyFillsBlankKind(t *testing.T) {
	record := MemoryRecord{}
	record.SetWorkingDefault()
	if record.Kind != RecordKindWorking {
		t.Fatalf("SetWorkingDefault blank kind = %q, want %q", record.Kind, RecordKindWorking)
	}

	record = MemoryRecord{Kind: RecordKindSemantic}
	record.SetWorkingDefault()
	if record.Kind != RecordKindSemantic {
		t.Fatalf("SetWorkingDefault existing kind = %q, want semantic preserved", record.Kind)
	}
}
