package plugin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCachedStateTracksBoundedMetadataRefresh(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			var asks atomic.Int32
			var spec string
			refreshSandbox(t, func(ctx context.Context, epoch uint64) error {
				asks.Add(1)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-release:
				}
				if fail {
					return errors.New("fixture listing failed")
				}
				commitProviders([]Provider{{ID: "fakeco", Spec: spec, Name: "Fresh Friendly Name"}}, &epoch)
				return nil
			})
			t.Cleanup(unblock)
			spec = refreshAbsPath(t, "testdata/fake/index.js")
			was := refreshTries
			refreshTries = 1
			t.Cleanup(func() { refreshTries = was })
			ps, answered, pending := CachedState()
			if len(ps) != 1 || ps[0].Name != "FakeCo" || answered || !pending {
				t.Fatalf("stale installed metadata gives no pending refresh hint: providers=%+v answered=%v pending=%v", ps, answered, pending)
			}
			unblock()
			Refreshed()
			ps, answered, pending = CachedState()
			if pending || answered == fail || asks.Load() != 1 {
				t.Fatalf("finished listing still asks the GUI to poll: answered=%v pending=%v asks=%d", answered, pending, asks.Load())
			}
			want := "Fresh Friendly Name"
			if fail {
				want = "FakeCo"
			}
			if len(ps) != 1 || ps[0].Name != want {
				t.Fatalf("listing lost the correct friendly name: %+v", ps)
			}
		})
	}
}

// An empty providers list means "the plugins haven't been asked yet" or "they
// answered, and this plugin signs in to nothing", and only the second is what
// the Installed list may say out loud (#1112). Answered tells them apart.
func TestAnsweredSaysWhetherTheListIsThePluginsOwn(t *testing.T) {
	sandbox(t)

	// what is kept from before they were last asked: not the answer
	UseCached(nil)
	if Answered() {
		t.Error("Answered = true with nothing asked: an empty list is not the plugins' answer")
	}
	provMu.Lock()
	provGood, provTried = false, false
	provMu.Unlock()
	if Answered() {
		t.Error("Answered = true after the list was forgotten, want false until the plugins answer")
	}

	// one asked about, and it has none: the answer is in, so a page may say so
	UseCached([]Provider{})
	provMu.Lock()
	provGood, provTried = true, true
	provMu.Unlock()
	if !Answered() {
		t.Error("Answered = false with an answer in hand, want true")
	}

	// asked, then the list forgotten again (a sign-in or the plugins changed)
	forgetProviders()
	if Answered() {
		t.Error("Answered = true once the list was invalidated, want false until the plugins answer again")
	}
}
