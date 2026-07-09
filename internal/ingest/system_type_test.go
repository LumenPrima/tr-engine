package ingest

import "testing"

func TestInferSystemTypeFromCall(t *testing.T) {
	tests := []struct {
		name string
		call CallData
		want string
	}{
		{
			name: "trunked digital p25",
			call: CallData{Conventional: false, Analog: false, AudioType: "digital"},
			want: "p25",
		},
		{
			name: "trunked phase2",
			call: CallData{Conventional: false, Analog: false, Phase2TDMA: true},
			want: "p25",
		},
		{
			name: "conventional digital",
			call: CallData{Conventional: true, Analog: false, AudioType: "digital"},
			want: "conventionalP25",
		},
		{
			name: "conventional p25 audio type",
			call: CallData{Conventional: true, Analog: false, AudioType: "P25"},
			want: "conventionalP25",
		},
		{
			name: "conventional phase2",
			call: CallData{Conventional: true, Analog: false, Phase2TDMA: true},
			want: "conventionalP25",
		},
		{
			name: "analog conventional no promotion",
			call: CallData{Conventional: true, Analog: true, AudioType: "analog"},
			want: "",
		},
		{
			name: "nil-safe empty",
			call: CallData{},
			// zero-value: conventional=false, analog=false → trunked digital signal
			want: "p25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferSystemTypeFromCall(&tt.call)
			if got != tt.want {
				t.Fatalf("inferSystemTypeFromCall() = %q, want %q", got, tt.want)
			}
		})
	}

	if got := inferSystemTypeFromCall(nil); got != "" {
		t.Fatalf("nil call = %q, want empty", got)
	}
}
