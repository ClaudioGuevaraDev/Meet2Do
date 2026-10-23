package ai

import "testing"

func TestClean(t *testing.T) {
	cases := map[string]string{
		"# TODO\n- [ ] a":                   "# TODO\n- [ ] a\n",
		"```markdown\n# TODO\n- [ ] a\n```": "# TODO\n- [ ] a\n",
		"  \n# TODO\n\n":                    "# TODO\n",
	}
	for in, want := range cases {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}
