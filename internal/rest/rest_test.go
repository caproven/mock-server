package rest

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticResponse(t *testing.T) {
	resp := Response{
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body:       []byte(`{"email":"johndoe@acme.com","title":"Staff Engineer"}`),
		StatusCode: http.StatusCreated,
		Delay:      3 * time.Second,
	}
	strategy := StaticResponse(resp)

	// Prove same response is returned each time
	for range 5 {
		got := strategy.NextResponse()
		assert.Equal(t, resp, got)
	}
}

func TestSequencedResponse(t *testing.T) {
	t.Run("nil sequence", func(t *testing.T) {
		strategy, err := NewSequencedResponse(SequenceBehaviorLoop, nil)
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("empty sequence", func(t *testing.T) {
		strategy, err := NewSequencedResponse(SequenceBehaviorLoop, []Response{})
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("loop with sequence length 1", func(t *testing.T) {
		resp := Response{
			StatusCode: http.StatusNotFound,
		}
		strategy, err := NewSequencedResponse(SequenceBehaviorLoop, []Response{resp})
		require.NoError(t, err)
		require.NotNil(t, strategy)

		for range 5 {
			assert.Equal(t, resp, strategy.NextResponse())
		}
	})

	t.Run("repeat last with sequence length 1", func(t *testing.T) {
		resp := Response{
			StatusCode: http.StatusGatewayTimeout,
			Body:       []byte("gateway timed out"),
		}
		strategy, err := NewSequencedResponse(SequenceBehaviorRepeatLast, []Response{resp})
		require.NoError(t, err)
		require.NotNil(t, strategy)

		for range 5 {
			assert.Equal(t, resp, strategy.NextResponse())
		}
	})

	t.Run("loop with multiple responses", func(t *testing.T) {
		first := Response{
			StatusCode: http.StatusOK,
		}
		second := Response{
			StatusCode: http.StatusNotFound,
		}
		strategy, err := NewSequencedResponse(SequenceBehaviorLoop, []Response{first, second})
		require.NoError(t, err)
		require.NotNil(t, strategy)

		for i := range 10 {
			got := strategy.NextResponse()
			if i%2 == 0 {
				assert.Equal(t, first, got)
			} else {
				assert.Equal(t, second, got)
			}
		}
	})

	t.Run("repeat last with multiple responses", func(t *testing.T) {
		first := Response{
			Body: []byte("first response"),
		}
		second := Response{
			Body: []byte("second response"),
		}
		third := Response{
			Body: []byte("third response"),
		}
		responses := []Response{first, second, third}
		strategy, err := NewSequencedResponse(SequenceBehaviorRepeatLast, responses)
		require.NoError(t, err)
		require.NotNil(t, strategy)

		assert.Equal(t, first, strategy.NextResponse())
		assert.Equal(t, second, strategy.NextResponse())
		for range 5 {
			assert.Equal(t, third, strategy.NextResponse())
		}
	})
}

type mockNumGenerator struct {
	val int
}

func (f *mockNumGenerator) N(_ int) int {
	return f.val
}

func TestWeightedResponse(t *testing.T) {
	t.Run("nil responses", func(t *testing.T) {
		strategy, err := NewWeightedResponse(nil, nil)
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("empty responses", func(t *testing.T) {
		strategy, err := NewWeightedResponse([]WeightedResponseEntry{}, nil)
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("default rng", func(t *testing.T) {
		entries := []WeightedResponseEntry{
			{
				Response: Response{
					StatusCode: http.StatusOK,
				},
				Weight: 5,
			},
			{
				Response: Response{
					StatusCode: http.StatusTeapot,
				},
				Weight: 1,
			},
		}
		strategy, err := NewWeightedResponse(entries, nil)
		require.NoError(t, err)
		require.NotNil(t, strategy)

		// Don't make assertions around rng but verify that *something* is returned
		for range 10 {
			assert.NotZero(t, strategy.NextResponse())
		}
	})

	t.Run("invalid 0 weight", func(t *testing.T) {
		entries := []WeightedResponseEntry{
			{
				Response: Response{
					Body: []byte("has zero weight"),
				},
				Weight: 0,
			},
		}
		strategy, err := NewWeightedResponse(entries, nil)
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("invalid negative weight", func(t *testing.T) {
		entries := []WeightedResponseEntry{
			{
				Response: Response{
					Body: []byte("has negative weight"),
				},
				Weight: -3,
			},
		}
		strategy, err := NewWeightedResponse(entries, nil)
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("single response", func(t *testing.T) {
		resp := Response{
			StatusCode: http.StatusOK,
			Body:       []byte("foo bar baz"),
		}
		weight := 5
		entries := []WeightedResponseEntry{
			{
				Response: resp,
				Weight:   weight,
			},
		}
		numberGen := &mockNumGenerator{}
		strategy, err := NewWeightedResponse(entries, numberGen)
		require.NoError(t, err)
		require.NotNil(t, strategy)

		// for all possible weight values, same resp is returned
		for i := range weight {
			numberGen.val = i
			got := strategy.NextResponse()
			assert.Equal(t, resp, got)
		}
	})

	t.Run("multiple responses", func(t *testing.T) {
		entries := []WeightedResponseEntry{
			{
				Response: Response{
					StatusCode: http.StatusOK,
					Body:       []byte("resp 1"),
				},
				Weight: 3,
			},
			{
				Response: Response{
					StatusCode: http.StatusBadRequest,
					Body:       []byte("resp 2"),
				},
				Weight: 1,
			},
			{
				Response: Response{
					StatusCode: http.StatusConflict,
					Body:       []byte("resp 3"),
				},
				Weight: 2,
			},
		}
		numberGen := &mockNumGenerator{}
		strategy, err := NewWeightedResponse(entries, numberGen)
		require.NoError(t, err)
		require.NotNil(t, strategy)

		var i int
		for _, entry := range entries {
			for range entry.Weight {
				numberGen.val = i
				got := strategy.NextResponse()
				assert.Equal(t, entry.Response, got)
				i++
			}
		}
	})

	t.Run("panics when invariant broken", func(t *testing.T) {
		entries := []WeightedResponseEntry{
			{
				Response: Response{
					StatusCode: http.StatusOK,
				},
				Weight: 1,
			},
		}
		numberGen := &mockNumGenerator{val: 1}
		strategy, err := NewWeightedResponse(entries, numberGen)
		require.NoError(t, err)
		require.NotNil(t, strategy)

		assert.Panics(t, func() {
			_ = strategy.NextResponse()
		})
	})
}
