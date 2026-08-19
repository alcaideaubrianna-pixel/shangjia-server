package sys

import "testing"

func TestBrowserVideoCompatible(t *testing.T) {
	tests := []struct {
		name  string
		ext   string
		codec string
		want  bool
	}{
		{name: "h264 mp4", ext: ".mp4", codec: "h264", want: true},
		{name: "hevc mov", ext: ".mov", codec: "hevc", want: false},
		{name: "hevc mp4", ext: ".mp4", codec: "hevc", want: false},
		{name: "vp9 webm", ext: ".webm", codec: "vp9", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := &browserVideoProbe{}
			probe.Streams = append(probe.Streams, struct {
				CodecName string `json:"codec_name"`
				CodecType string `json:"codec_type"`
			}{CodecName: test.codec, CodecType: "video"})
			if got := browserVideoCompatible(probe, test.ext); got != test.want {
				t.Fatalf("browserVideoCompatible() = %v, want %v", got, test.want)
			}
		})
	}
}
