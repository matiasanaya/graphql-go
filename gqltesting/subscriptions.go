package gqltesting

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"testing"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/errors"
)

// TestResponse models the expected response
type TestResponse struct {
	Data   json.RawMessage
	Errors []*errors.QueryError
}

// TestSubscription is a GraphQL test case to be used with RunSubscribe.
type TestSubscription struct {
	Name            string
	Context         context.Context
	Schema          *graphql.Schema
	Query           string
	OperationName   string
	Variables       map[string]interface{}
	ExpectedResults []TestResponse
}

// RunSubscribes runs the given GraphQL subscription test cases as subtests.
func RunSubscribes(t *testing.T, tests []*TestSubscription) {
	for i, test := range tests {
		if test.Name == "" {
			test.Name = strconv.Itoa(i + 1)
		}

		t.Run(test.Name, func(t *testing.T) {
			RunSubscribe(t, test)
		})
	}
}

// RunSubscribe runs a single GraphQL subscription test case.
func RunSubscribe(t *testing.T, test *TestSubscription) {
	if test.Context == nil {
		test.Context = context.Background()
	}
	c := test.Schema.Subscribe(test.Context, test.Query, test.OperationName, test.Variables)

	var results []*graphql.Response
	for res := range c {
		results = append(results, res)
	}

	for i, expected := range test.ExpectedResults {
		res := results[i]

		checkErrorStrings(t, expected.Errors, res.Errors)

		got := formatJSON(t, res.Data)
		want := formatJSON(t, []byte(expected.Data))

		if !bytes.Equal(got, want) {
			t.Logf("got:  %s", got)
			t.Logf("want: %s", want)
			t.Fail()
		}
	}
}

func checkErrorStrings(t *testing.T, expected, actual []*errors.QueryError) {
	expectedCount, actualCount := len(expected), len(actual)

	if expectedCount != actualCount {
		t.Fatalf("unexpected number of errors: want %d, got %d", expectedCount, actualCount)
	}

	if expectedCount > 0 {
		for i, want := range expected {
			got := actual[i]

			if got.Error() != want.Error() {
				t.Fatalf("unexpected error: got %+v, want %+v", got, want)
			}
		}

		// Return because we're done checking.
		return
	}

	for _, err := range actual {
		t.Errorf("unexpected error: '%s'", err)
	}
}
