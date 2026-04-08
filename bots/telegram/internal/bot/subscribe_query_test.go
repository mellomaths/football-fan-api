package bot

import "testing"

func TestSubscribeTeamQueryFromMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "subscribe_and_team",
			text:     "/subscribe Flamengo",
			expected: "Flamengo",
		},
		{
			name:     "subscribe_at_bot",
			text:     "/subscribe@FootballFanBot Flamengo",
			expected: "Flamengo",
		},
		{
			name:     "case_insensitive_command",
			text:     "/SUBSCRIBE Palmeiras",
			expected: "Palmeiras",
		},
		{
			name:     "multi_word_team",
			text:     "/subscribe Atlético Mineiro",
			expected: "Atlético Mineiro",
		},
		{
			name:     "only_command",
			text:     "/subscribe",
			expected: "",
		},
		{
			name:     "nil_empty",
			text:     "",
			expected: "",
		},
		{
			name:     "extra_spaces_after_command",
			text:     "/subscribe   Flamengo",
			expected: "Flamengo",
		},
		{
			name:     "thin_space_after_command_trimmed_with_query",
			text:     "/subscribe\u2009Flamengo",
			expected: "Flamengo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := subscribeTeamQueryFromMessage(tt.text)
			if got != tt.expected {
				t.Fatalf("subscribeTeamQueryFromMessage(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}
