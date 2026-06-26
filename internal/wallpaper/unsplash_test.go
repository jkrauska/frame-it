package wallpaper

import "testing"

func TestUnsplashDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		photo unsplashPhoto
		want  string
	}{
		{
			name: "prefers alt description",
			photo: unsplashPhoto{
				Description:    "If you find my photos useful, please consider subscribing to me on YouTube",
				AltDescription: "ocean waves at dusk",
			},
			want: "ocean waves at dusk",
		},
		{
			name: "falls back to useful description",
			photo: unsplashPhoto{
				Description: "Sunset over the ocean",
			},
			want: "Sunset over the ocean",
		},
		{
			name: "uses alt when description is promo",
			photo: unsplashPhoto{
				Description:    "Follow me on Instagram for more!",
				AltDescription: "green forest trail",
			},
			want: "green forest trail",
		},
		{
			name: "drops promo-only description",
			photo: unsplashPhoto{
				Description: "If you find my photos useful, please consider subscribing to me on YouTube for the occasional photography tutorial",
			},
			want: "",
		},
		{
			name:  "empty",
			photo: unsplashPhoto{},
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := unsplashDescription(tc.photo); got != tc.want {
				t.Fatalf("unsplashDescription() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsUsefulCaption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want bool
	}{
		{"Mountain lake at sunrise", true},
		{"If you find my photos useful, please consider subscribing", false},
		{"Follow me on Instagram", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := isUsefulCaption(tc.text); got != tc.want {
			t.Fatalf("isUsefulCaption(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
