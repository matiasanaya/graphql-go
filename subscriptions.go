package graphql

import (
	"context"

	"github.com/graph-gophers/graphql-go/errors"
	"github.com/graph-gophers/graphql-go/internal/common"
	"github.com/graph-gophers/graphql-go/internal/exec"
	"github.com/graph-gophers/graphql-go/internal/exec/resolvable"
	"github.com/graph-gophers/graphql-go/internal/exec/selected"
	"github.com/graph-gophers/graphql-go/internal/query"
	"github.com/graph-gophers/graphql-go/internal/validation"
	"github.com/graph-gophers/graphql-go/introspection"
)

func (s *Schema) Subscribe(ctx context.Context, queryString string, operationName string, variables map[string]interface{}) chan *Response {
	if s.res == nil {
		panic("schema created without resolver, can not subscribe")
	}
	return s.subscribe(ctx, queryString, operationName, variables, s.res)
}

func sendAndReturnClosed(resp *Response) chan *Response {
	c := make(chan *Response, 1)
	c <- resp
	close(c)
	return c
}

func (s *Schema) subscribe(ctx context.Context, queryString string, operationName string, variables map[string]interface{}, res *resolvable.Schema) chan *Response {
	doc, qErr := query.Parse(queryString)
	if qErr != nil {
		return sendAndReturnClosed(&Response{Errors: []*errors.QueryError{qErr}})
	}

	validationFinish := s.validationTracer.TraceValidation()
	errs := validation.Validate(s.schema, doc, s.maxDepth)
	validationFinish(errs)
	if len(errs) != 0 {
		return sendAndReturnClosed(&Response{Errors: errs})
	}

	op, err := getOperation(doc, operationName)
	if err != nil {
		return sendAndReturnClosed(&Response{Errors: []*errors.QueryError{errors.Errorf("%s", err)}})
	}

	r := &exec.Request{
		Request: selected.Request{
			Doc:    doc,
			Vars:   variables,
			Schema: s.schema,
		},
		Limiter: make(chan struct{}, s.maxParallelism),
		Tracer:  s.tracer,
		Logger:  s.logger,
	}
	varTypes := make(map[string]*introspection.Type)
	for _, v := range op.Vars {
		t, err := common.ResolveType(v.Type, s.schema.Resolve)
		if err != nil {
			return sendAndReturnClosed(&Response{Errors: []*errors.QueryError{err}})
		}
		varTypes[v.Name.Name] = introspection.WrapType(t)
	}

	traceCtx, finish := s.tracer.TraceQuery(ctx, queryString, operationName, variables, varTypes)
	responses := r.Subscribe(traceCtx, res, op)
	finish(nil)

	c := make(chan *Response)
	go func() {
		for resp := range responses {
			c <- &Response{
				Data:   resp.Bytes,
				Errors: resp.Errs,
			}
		}
		close(c)
	}()

	return c
}
