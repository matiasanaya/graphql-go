package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/graph-gophers/graphql-go/errors"
	"github.com/graph-gophers/graphql-go/internal/exec/resolvable"
	"github.com/graph-gophers/graphql-go/internal/exec/selected"
	"github.com/graph-gophers/graphql-go/internal/query"
)

type Response struct {
	Data json.RawMessage
	Errs []*errors.QueryError
}

func (r *Request) Subscribe(ctx context.Context, s *resolvable.Schema, op *query.Operation) <-chan *Response {
	if op.Type != query.Subscription {
		return sendAndReturnClosed(&Response{Errs: []*errors.QueryError{errors.Errorf("%s: %s", "subscription unavailable for operation of type", op.Type)}})
	}

	var result reflect.Value
	var f *fieldToExec
	var qErr *errors.QueryError
	func() {
		defer r.handlePanic(ctx)

		sels := selected.ApplyOperation(&r.Request, s, op)
		var fields []*fieldToExec
		collectFieldsToResolve(sels, s.Resolver, &fields, make(map[string]*fieldToExec))

		// TODO: more subscriptions at once
		f = fields[0]

		var in []reflect.Value
		if f.field.HasContext {
			in = append(in, reflect.ValueOf(ctx))
		}
		if f.field.ArgsPacker != nil {
			in = append(in, f.field.PackedArgs)
		}
		callOut := f.resolver.Method(f.field.MethodIndex).Call(in)
		result = callOut[0]
		if f.field.HasError && !callOut[1].IsNil() {
			resolverErr := callOut[1].Interface().(error)
			qErr = errors.Errorf("%s", resolverErr)
			qErr.ResolverError = resolverErr
		}
	}()

	if qErr != nil {
		return sendAndReturnClosed(&Response{Errs: []*errors.QueryError{qErr}})
	}

	if err := ctx.Err(); err != nil {
		return sendAndReturnClosed(&Response{Errs: []*errors.QueryError{errors.Errorf("%s", err)}})
	}

	c := make(chan *Response)
	// TODO: handle resolver nil channel better?
	if result == reflect.Zero(result.Type()) {
		close(c)
		return c
	}

	go func() {
		wasClosed := false
		for {
			ctx := context.Background()
			func() {
				defer r.handlePanic(ctx)
				obj, ok := result.Recv()
				if !ok {
					wasClosed = true
					close(c)
					return
				}
				var out bytes.Buffer
				out.WriteString(fmt.Sprintf(`{"%s":`, f.field.Alias))
				r.execSelectionSet(ctx, f.sels, f.field.Type, &pathSegment{nil, f.field.Alias}, obj, &out)
				out.WriteString(`}`)
				c <- &Response{Data: out.Bytes()}
			}()
			if err := ctx.Err(); err != nil {
				c <- &Response{Errs: []*errors.QueryError{errors.Errorf("%s", err)}}
			}
			if wasClosed {
				return
			}
		}
	}()

	return c
}

func sendAndReturnClosed(resp *Response) chan *Response {
	c := make(chan *Response, 1)
	c <- resp
	close(c)
	return c
}
