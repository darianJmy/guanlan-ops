package syncbuffer

import (
	"strings"
	"sync"
	"testing"
)

func TestConcurrentOutputAndLimit(t *testing.T) {
	for _, limit := range []int{0, 73} {
		b := Buffer{Limit: limit}
		var wg sync.WaitGroup
		for i := 0; i < 40; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				n, err := b.Write([]byte("hello"))
				if n != 5 || err != nil {
					t.Errorf("write: %d %v", n, err)
				}
				_ = b.String()
			}()
		}
		wg.Wait()
		want := strings.Repeat("hello", 40)
		if limit > 0 {
			want = want[:limit]
		}
		if b.String() != want {
			t.Errorf("unexpected output length %d", len(b.String()))
		}
	}
}
