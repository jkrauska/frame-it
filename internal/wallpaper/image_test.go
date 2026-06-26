package wallpaper

import "testing"

func TestImageCaption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  Image
		want string
	}{
		{
			name: "description and author",
			img: Image{
				Description: "green grass field during sunset",
				Credit:      "Robert Lukeman (https://unsplash.com/@robertlukeman)",
			},
			want: "green grass field during sunset · Robert Lukeman",
		},
		{
			name: "author only",
			img: Image{
				Credit: "Jane Doe",
			},
			want: "Jane Doe",
		},
		{
			name: "long description keeps author",
			img: Image{
				Description: "It was a cloudy day and I was on my couch, bored because I was doing nothing, so at 4pm I decided to go to Lake Carezza. The weather was not good, but according to the forecast it would be better for the sunset. It was worth it.",
				Credit:      "Robert Lukeman",
			},
			want: "... · Robert Lukeman",
		},
		{
			name: "empty",
			img:  Image{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.img.Caption(); got != tc.want {
				t.Fatalf("Caption() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthorName(t *testing.T) {
	t.Parallel()

	if got := authorName("Robert Lukeman (https://unsplash.com/@robertlukeman)"); got != "Robert Lukeman" {
		t.Fatalf("authorName() = %q, want Robert Lukeman", got)
	}
	if got := authorName("Jane Doe"); got != "Jane Doe" {
		t.Fatalf("authorName() = %q, want Jane Doe", got)
	}
}
