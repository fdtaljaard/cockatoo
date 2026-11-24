package mail

import "testing"

func TestParseUnsubscribe(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Unsubscribe
	}{
		{"none", "", Unsubscribe{}},
		{"https and mailto, one-click",
			"List-Unsubscribe: <mailto:leave@lists.example.com?subject=unsub>, <https://example.com/u?id=1>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n",
			Unsubscribe{URL: "https://example.com/u?id=1", Mailto: "mailto:leave@lists.example.com?subject=unsub", OneClick: true}},
		{"folded over lines",
			"List-Unsubscribe:\r\n <https://example.com/a/very/long/path>,\r\n\t<mailto:x@example.com>\r\n",
			Unsubscribe{URL: "https://example.com/a/very/long/path", Mailto: "mailto:x@example.com"}},
		{"mailto only", "List-Unsubscribe: <mailto:off@example.com>\r\n", Unsubscribe{Mailto: "mailto:off@example.com"}},
		{"plain http is not kept", "List-Unsubscribe: <http://example.com/u>\r\n", Unsubscribe{}},
		{"javascript and data are not kept", "List-Unsubscribe: <javascript:alert(1)>, <data:text/html,hi>\r\n", Unsubscribe{}},
		{"no angle brackets", "List-Unsubscribe: https://example.com/u\r\n", Unsubscribe{}},
		{"garbage", "List-Unsubscribe: <<<>>> ,, <%zz>\r\n", Unsubscribe{}},
		{"one-click needs an https address", "List-Unsubscribe: <mailto:a@b.c>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n",
			Unsubscribe{Mailto: "mailto:a@b.c"}},
		{"first https wins", "List-Unsubscribe: <https://a.example/1>, <https://b.example/2>\r\n", Unsubscribe{URL: "https://a.example/1"}},
	}
	for _, c := range cases {
		if got := parseUnsubscribe([]byte(c.raw)); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestChooseFolder(t *testing.T) {
	list := []Folder{
		{Name: "INBOX"},
		{Name: "[Gmail]/All Mail", Attrs: []string{`\All`}},
		{Name: "[Gmail]/Papierkorb", Attrs: []string{`\HasNoChildren`, `\Trash`}},
		{Name: "[Gmail]/Spam", Attrs: []string{`\Junk`}},
		{Name: "Trash"}, // a user's own label: the attribute wins over the name
	}
	if got, err := chooseFolder(list, Trash); err != nil || got != "[Gmail]/Papierkorb" {
		t.Errorf("trash by attribute: %q %v", got, err)
	}
	if got, err := chooseFolder(list, Spam); err != nil || got != "[Gmail]/Spam" {
		t.Errorf("spam by attribute: %q %v", got, err)
	}

	// Without attributes, the known names, localised ones too.
	byName := []Folder{{Name: "INBOX"}, {Name: "[Google Mail]/Bin"}, {Name: "[Google Mail]/Spam"}}
	if got, err := chooseFolder(byName, Trash); err != nil || got != "[Google Mail]/Bin" {
		t.Errorf("trash by name: %q %v", got, err)
	}
	if got, err := chooseFolder(byName, Spam); err != nil || got != "[Google Mail]/Spam" {
		t.Errorf("spam by name: %q %v", got, err)
	}

	// Nothing matches: refuse, never guess or create.
	if got, err := chooseFolder([]Folder{{Name: "INBOX"}, {Name: "Receipts"}}, Trash); err == nil {
		t.Errorf("no trash: chose %q", got)
	}
}
