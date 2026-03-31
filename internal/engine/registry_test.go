package engine

import (
	"sync"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestRegistry_GetRegisteredBlock(t *testing.T) {
	r := NewRegistry()
	blk := &fakeBlock{typ: "http"}
	r.Register(blk)

	got, err := r.Get(model.BlockType("http"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != blk {
		t.Fatal("returned block does not match registered block")
	}
}

func TestRegistry_GetUnknownBlock(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get(model.BlockType("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown block type, got nil")
	}
}

func TestRegistry_RegisterNilIsIgnored(t *testing.T) {
	r := NewRegistry()
	r.Register(nil)

	all := r.All()
	if len(all) != 0 {
		t.Fatalf("expected empty registry, got %d blocks", len(all))
	}
}

func TestRegistry_RegisterOverwritesExisting(t *testing.T) {
	r := NewRegistry()
	first := &fakeBlock{typ: "grpc"}
	second := &fakeBlock{typ: "grpc"}
	r.Register(first)
	r.Register(second)

	got, err := r.Get(model.BlockType("grpc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != second {
		t.Fatal("expected second registration to overwrite first")
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeBlock{typ: "http"})
	r.Register(&fakeBlock{typ: "grpc"})
	r.Register(&fakeBlock{typ: "postgres"})

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(all))
	}
}

func TestRegistry_CloseCallsCloser(t *testing.T) {
	r := NewRegistry()
	closed := false
	r.Register(&fakeClosableBlock{fakeBlock: fakeBlock{typ: "grpc"}, onClose: func() { closed = true }})
	r.Close()

	if !closed {
		t.Fatal("expected Close to be called on closable block")
	}
}

func TestRegistry_CloseSkipsNonCloser(_ *testing.T) {
	r := NewRegistry()
	r.Register(&fakeBlock{typ: "http"})
	// should not panic
	r.Close()
}

func TestRegistry_ConcurrentRegisterAndGet(_ *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(2)
		typ := fakeBlockType(i)
		go func() {
			defer wg.Done()
			r.Register(&fakeBlock{typ: typ})
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Get(typ)
		}()
	}
	wg.Wait()
}

func fakeBlockType(i int) model.BlockType {
	return model.BlockType("block-" + string(rune('a'+i%26)))
}

type fakeClosableBlock struct {
	fakeBlock
	onClose func()
}

func (b *fakeClosableBlock) Close() {
	b.onClose()
}
