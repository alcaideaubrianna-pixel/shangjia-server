package sysin

import (
	"context"
	"testing"
)

func TestProfileCycleOverrideFilter(t *testing.T) {
	tests := []struct {
		name    string
		in      ProfileCycleOverrideInp
		wantErr bool
	}{
		{
			name:    "missing selector",
			in:      ProfileCycleOverrideInp{CycleOverrideEnabled: true, CycleOverrideDays: 3},
			wantErr: true,
		},
		{
			name: "blank selectors",
			in: ProfileCycleOverrideInp{
				ProfileViewInp: ProfileViewInp{Uuid: " \t", ProfileNo: "\n "},
			},
			wantErr: true,
		},
		{
			name: "uuid selector",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:       ProfileViewInp{Uuid: "83dc7311-6c20-42b0-b120-a65314c8d99d"},
				CycleOverrideEnabled: true, CycleOverrideDays: 3,
			},
		},
		{
			name: "id selector",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:       ProfileViewInp{Id: 42},
				CycleOverrideEnabled: true, CycleOverrideDays: 3,
			},
		},
		{
			name: "profile number selector",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:       ProfileViewInp{ProfileNo: "P42"},
				CycleOverrideEnabled: true, CycleOverrideDays: 3,
			},
		},
		{
			name: "enabled zero days",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:       ProfileViewInp{Id: 42},
				CycleOverrideEnabled: true,
			},
			wantErr: true,
		},
		{
			name: "enabled negative days",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:       ProfileViewInp{Id: 42},
				CycleOverrideEnabled: true, CycleOverrideDays: -1,
			},
			wantErr: true,
		},
		{
			name: "disabled zero preserves sentinel",
			in: ProfileCycleOverrideInp{
				ProfileViewInp: ProfileViewInp{Id: 42},
			},
		},
		{
			name: "enabled minimum positive days",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:       ProfileViewInp{Id: 42},
				CycleOverrideEnabled: true, CycleOverrideDays: 1,
			},
		},
		{
			name: "disabled positive days",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:    ProfileViewInp{Id: 42},
				CycleOverrideDays: 7,
			},
		},
		{
			name: "disabled negative days",
			in: ProfileCycleOverrideInp{
				ProfileViewInp:    ProfileViewInp{Id: 42},
				CycleOverrideDays: -1,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			err := in.Filter(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Filter() error = %v, wantErr %v", err, tt.wantErr)
			}
			// In particular, disabled zero must survive validation so saving can
			// retain the stored interval instead of replacing it with a default.
			if in != tt.in {
				t.Fatalf("Filter() changed input: got %+v, want %+v", in, tt.in)
			}
		})
	}
}
