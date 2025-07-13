package rest

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResponse(t *testing.T) {
	cases := map[string]struct {
		opts    []ResponseOption
		want    Response
		wantErr bool
	}{
		"all defaults": {
			want: Response{
				statusCode: http.StatusOK,
			},
		},
		"status code too low": {
			opts: []ResponseOption{
				WithResponseStatus(0),
			},
			wantErr: true,
		},
		"status code too high": {
			opts: []ResponseOption{
				WithResponseStatus(600),
			},
			wantErr: true,
		},
		"valid status code": {
			opts: []ResponseOption{
				WithResponseStatus(http.StatusTooManyRequests),
			},
			want: Response{
				statusCode: http.StatusTooManyRequests,
			},
		},
		"body": {
			opts: []ResponseOption{
				WithResponseBody([]byte("user created")),
			},
			want: Response{
				statusCode: http.StatusOK,
				body:       []byte("user created"),
			},
		},
		"headers": {
			opts: []ResponseOption{
				WithResponseHeaders(map[string]string{
					"Content-Type":         "application/json",
					"X-Remaining-Requests": "50",
				}),
			},
			want: Response{
				statusCode: http.StatusOK,
				headers: map[string]string{
					"Content-Type":         "application/json",
					"X-Remaining-Requests": "50",
				},
			},
		},
		"negative delay": {
			opts: []ResponseOption{
				WithResponseDelay(-1 * time.Second),
			},
			wantErr: true,
		},
		"zero delay": {
			opts: []ResponseOption{
				WithResponseDelay(0),
			},
			want: Response{
				statusCode: http.StatusOK,
			},
		},
		"positive delay": {
			opts: []ResponseOption{
				WithResponseDelay(5 * time.Second),
			},
			want: Response{
				statusCode: http.StatusOK,
				delay:      5 * time.Second,
			},
		},
		"composite": {
			opts: []ResponseOption{
				WithResponseStatus(http.StatusCreated),
				WithResponseHeaders(map[string]string{
					"Content-Type": "application/json",
				}),
				WithResponseBody([]byte(`{"id":"qwerty1234"}`)),
				WithResponseDelay(50 * time.Millisecond),
			},
			want: Response{
				headers: map[string]string{
					"Content-Type": "application/json",
				},
				body:       []byte(`{"id":"qwerty1234"}`),
				statusCode: http.StatusCreated,
				delay:      50 * time.Millisecond,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := NewResponse(tc.opts...)
			if tc.wantErr {
				require.Error(t, err)
				require.Zero(t, resp)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, resp)
		})
	}
}

func TestStaticResponse(t *testing.T) {
	resp := Response{
		headers: map[string]string{
			"Content-Type": "application/json",
		},
		body:       []byte(`{"email":"johndoe@acme.com","title":"Staff Engineer"}`),
		statusCode: http.StatusCreated,
		delay:      3 * time.Second,
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

	t.Run("invalid end behavior", func(t *testing.T) {
		responses := []Response{
			{
				body: []byte("unused"),
			},
		}
		strategy, err := NewSequencedResponse(SequenceBehavior("invalid"), responses)
		assert.Error(t, err)
		assert.Nil(t, strategy)
	})

	t.Run("loop with sequence length 1", func(t *testing.T) {
		resp := Response{
			statusCode: http.StatusNotFound,
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
			statusCode: http.StatusGatewayTimeout,
			body:       []byte("gateway timed out"),
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
			statusCode: http.StatusOK,
		}
		second := Response{
			statusCode: http.StatusNotFound,
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
			body: []byte("first response"),
		}
		second := Response{
			body: []byte("second response"),
		}
		third := Response{
			body: []byte("third response"),
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
					statusCode: http.StatusOK,
				},
				Weight: 5,
			},
			{
				Response: Response{
					statusCode: http.StatusTeapot,
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
					body: []byte("has zero weight"),
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
					body: []byte("has negative weight"),
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
			statusCode: http.StatusOK,
			body:       []byte("foo bar baz"),
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
					statusCode: http.StatusOK,
					body:       []byte("resp 1"),
				},
				Weight: 3,
			},
			{
				Response: Response{
					statusCode: http.StatusBadRequest,
					body:       []byte("resp 2"),
				},
				Weight: 1,
			},
			{
				Response: Response{
					statusCode: http.StatusConflict,
					body:       []byte("resp 3"),
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
					statusCode: http.StatusOK,
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
