package bangchat

import "testing"

func TestAPIBaseFromSourceURL(t *testing.T) {
	tests := []struct {
		name      string
		sourceURL string
		want      string
	}{
		{
			name:      "http ip with port",
			sourceURL: "http://192.0.2.10:18080/chat?token=test-token",
			want:      "http://192.0.2.10:18080/api",
		},
		{
			name:      "https domain",
			sourceURL: "https://seats.bangchats.top/chat?token=test-token",
			want:      "https://seats.bangchats.top/api",
		},
		{
			name:      "raw token uses default api",
			sourceURL: "test-token",
			want:      apiBaseURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiBaseFromSourceURL(tt.sourceURL); got != tt.want {
				t.Fatalf("apiBaseFromSourceURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
