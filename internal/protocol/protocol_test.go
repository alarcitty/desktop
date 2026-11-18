package protocol

import "testing"

func TestIsDeepLink(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"mila://anime/52991", true},
		{"mila://anime/52991?ep=3", true},
		{"MILA://ANIME/52991", true},
		{"  mila://anime/alias/frieren  ", true},
		{`"mila://anime/52991"`, false},
		{"https://shikimori.io/animes/52991", false},
		{"mila://", true},
		{"", false},
		{"notmila://anime/1", false},
	}
	for _, c := range cases {
		if got := IsDeepLink(c.in); got != c.want {
			t.Errorf("IsDeepLink(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFindLink(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"empty", nil, ""},
		{"no link", []string{"--flag", "value"}, ""},
		{"plain", []string{"mila://anime/52991"}, "mila://anime/52991"},
		{"quoted", []string{`"mila://anime/52991?ep=3"`}, "mila://anime/52991?ep=3"},
		{"mixed args", []string{"--dev", "mila://anime/alias/frieren", "x"}, "mila://anime/alias/frieren"},
		{"first wins", []string{"mila://anime/1", "mila://anime/2"}, "mila://anime/1"},
		{"case insensitive", []string{"MILA://anime/7"}, "MILA://anime/7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FindLink(c.args); got != c.want {
				t.Errorf("FindLink(%v) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

func TestOpenCommand(t *testing.T) {
	got := OpenCommand(`C:\Program Files\Mila\Mila.exe`)
	want := `"C:\Program Files\Mila\Mila.exe" "%1"`
	if got != want {
		t.Errorf("OpenCommand = %q, want %q", got, want)
	}
}