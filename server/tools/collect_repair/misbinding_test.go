package main

import "testing"

func TestClassifyMisbindingCandidate(t *testing.T) {
	base := misbindingCandidate{ProfileID: 10, OldProfileID: 11, VerifyMedia: 1, ReadyVideos: 1, OldIndependent: 1}
	tests := []struct {
		name  string
		row   misbindingCandidate
		count int
		want  string
	}{
		{name: "safe", row: base, count: 1, want: "safe"},
		{name: "ambiguous", row: base, count: 2, want: "manual"},
		{name: "media not ready", row: func() misbindingCandidate { value := base; value.ReadyVideos = 0; return value }(), count: 1, want: "manual"},
		{name: "target already verified", row: func() misbindingCandidate { value := base; value.TargetVerifies = 1; return value }(), count: 1, want: "manual"},
		{name: "old profile would lose verify", row: func() misbindingCandidate { value := base; value.OldIndependent = 0; return value }(), count: 1, want: "manual"},
		{name: "same profile", row: func() misbindingCandidate { value := base; value.OldProfileID = value.ProfileID; return value }(), count: 1, want: "skip"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := classifyMisbindingCandidate(test.row, test.count, 1)
			if got != test.want {
				t.Fatalf("decision=%q want=%q", got, test.want)
			}
		})
	}
}

func TestClassifyMisbindingCandidateRejectsMultipleCandidatesForProfile(t *testing.T) {
	row := misbindingCandidate{ProfileID: 10, OldProfileID: 11, VerifyMedia: 1, ReadyVideos: 1, OldIndependent: 1}
	got, _ := classifyMisbindingCandidate(row, 1, 2)
	if got != "manual" {
		t.Fatalf("decision=%q want=manual", got)
	}
}

func TestClassifyMisbindingCandidateResumesLoggedRepair(t *testing.T) {
	row := misbindingCandidate{ProfileID: 10, OldProfileID: 11, VerifyMedia: 1, ReadyVideos: 1, TargetVerifies: 1, RepairLogged: true}
	got, _ := classifyMisbindingCandidate(row, 1, 1)
	if got != "safe" {
		t.Fatalf("decision=%q want=safe", got)
	}
}

func TestClassifyMisbindingCandidateSkipsExistingRepublish(t *testing.T) {
	row := misbindingCandidate{RepairLogged: true, RepublishJobs: 1}
	got, _ := classifyMisbindingCandidate(row, 1, 1)
	if got != "skip" {
		t.Fatalf("decision=%q want=skip", got)
	}
}
