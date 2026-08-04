package gitlab

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// badClient points at an endpoint that refuses connections, exercising the
// transport-error branch of do() for every method.
func badClient() *Client {
	return New("http://127.0.0.1:0", "tok", botUser)
}

func TestTransportErrors(t *testing.T) {
	c := badClient()
	if _, err := c.GetMR("42", "7"); err == nil {
		t.Error("GetMR: want transport error")
	}
	if _, err := c.FileAtRef("42", "f.yaml", "main"); err == nil {
		t.Error("FileAtRef: want transport error")
	}
	if _, err := c.ListBotThreads("42", "7"); err == nil {
		t.Error("ListBotThreads: want transport error")
	}
	if _, err := c.CreateThread("42", "7", botMarker(), "b"); err == nil {
		t.Error("CreateThread: want transport error")
	}
	if err := c.ResolveThread("42", "7", "d1"); err == nil {
		t.Error("ResolveThread: want transport error")
	}
	if _, err := c.Approve("42", "7"); err == nil {
		t.Error("Approve: want transport error")
	}
	if _, _, _, err := c.CurrentHeads("42", "7"); err == nil {
		t.Error("CurrentHeads: want transport error")
	}
	m := forge.DesiredMerge{SourceSha: "s", TargetSha: "t", MergeResultDigest: "d"}
		if _, err := c.MergeCAS("42", "7", m); err == nil {
		t.Error("MergeCAS: want transport error")
	}
	if _, err := c.Snapshot("42", "7"); err == nil {
		t.Error("Snapshot: want transport error")
	}
}

func TestGetMRUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := c.GetMR("42", "7"); err == nil {
		t.Fatal("want error on non-200 MR read")
	}
}

func TestGetMRDecodeError(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := c.GetMR("42", "7"); err == nil {
		t.Fatal("want decode error")
	}
}

func TestBranchTipUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "merge_requests") {
			_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"sha":"s","source_branch":"f","target_branch":"main"}`))
			return
		}
		http.Error(w, "no branch", http.StatusNotFound)
	})
	if _, err := c.GetMR("42", "7"); err == nil {
		t.Fatal("want error on branch read failure")
	}
}

func TestBranchTipDecodeError(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "merge_requests") {
			_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"sha":"s","source_branch":"f","target_branch":"main"}`))
			return
		}
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := c.GetMR("42", "7"); err == nil {
		t.Fatal("want branch decode error")
	}
}

func TestListBotThreadsUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := c.ListBotThreads("42", "7"); err == nil {
		t.Fatal("want error on non-200 discussions")
	}
}

func TestListBotThreadsDecodeError(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := c.ListBotThreads("42", "7"); err == nil {
		t.Fatal("want decode error")
	}
}

func TestListBotThreadsEmptyNotesSkipped(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`[{"id":"d","notes":[]}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})
	threads, err := c.ListBotThreads("42", "7")
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 0 {
		t.Fatalf("empty-notes discussion should be skipped, got %d", len(threads))
	}
}

func TestCreateThreadUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := c.CreateThread("42", "7", botMarker(), "b"); err == nil {
		t.Fatal("want error on non-201 create")
	}
}

func TestCreateThreadDecodeError(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := c.CreateThread("42", "7", botMarker(), "b"); err == nil {
		t.Fatal("want decode error")
	}
}

func TestResolveThreadUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if err := c.ResolveThread("42", "7", "d1"); err == nil {
		t.Fatal("want error on non-200 resolve")
	}
}

func TestApproveUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := c.Approve("42", "7"); err == nil {
		t.Fatal("want error on unexpected approve status")
	}
}
