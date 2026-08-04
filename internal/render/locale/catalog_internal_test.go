package locale

import "testing"

func TestMustLoadCatalogInvalidYAML(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid catalog YAML")
		}
	}()
	mustLoadCatalog("bad", []byte(":\n  - ["))
}
