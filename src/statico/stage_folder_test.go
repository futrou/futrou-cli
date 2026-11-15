package statico

import "testing"

func TestFolderNameKeepsCleanNamesAndSeparatesChangedOnes(t *testing.T) {
	if got := folderName("my_site"); got != "my_site" {
		t.Errorf("clean name changed to %q", got)
	}
	names := []string{"my site", "my_site", "a/b", "a_b", "..", "_"}
	seen := map[string]string{}
	for _, n := range names {
		f := folderName(n)
		if other, ok := seen[f]; ok {
			t.Errorf("%q and %q share the folder %q", n, other, f)
		}
		seen[f] = n
	}
}
