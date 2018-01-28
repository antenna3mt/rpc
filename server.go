// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2012 The Gorilla Authors. All rights reserved.
// Copyright 2018 Yi Jin. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc

import (
	"strings"
	"net/http"
	"fmt"
	"reflect"
)

/*
NewServer returns a new RPC server.
param ctx is non-nil, and used to restrict the context param for service registering
*/
func NewServer(ctx interface{}) (*Server, error) {
	if ctx == nil {
		return nil, fmt.Errorf("rpc: ctx is nil")
	}
	ctxType := reflect.TypeOf(ctx)
	if ctxType.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("rpc: ctx is not pointer")
	}

	return &Server{
		codecs:   make(map[string]Codec),
		services: new(serviceMap),
		ctxType:  ctxType.Elem(),
	}, nil
}

/*
serves registered services with registered codecs.
 */
type Server struct {
	codecs    map[string]Codec // codecs
	services  *serviceMap      // services
	ctxType   reflect.Type     // context type
	beforeFns []reflect.Value  // functions executed before service call
	afterFns  []reflect.Value  // functions executed after service all
}

/*
RegisterBeforeFunc validate and add a func that will be executed before service call
*/
func (s *Server) RegisterBeforeFunc(fn interface{}) error {
	if err := validCtxFunc(fn, s.ctxType); err != nil {
		return err
	}
	s.beforeFns = append(s.beforeFns, reflect.ValueOf(fn))
	return nil
}

/*
RegisterAfterFunc validate and add a func that will be executed after service call
 */
func (s *Server) RegisterAfterFunc(fn interface{}) error {
	if err := validCtxFunc(fn, s.ctxType); err != nil {
		return err
	}
	s.afterFns = append(s.beforeFns, reflect.ValueOf(fn))
	return nil
}

/*
RegisterCodec adds a new codec to the server.

Codecs are defined to process a given serialization scheme, e.g., JSON or
XML. A codec is chosen based on the "Content-Type" header from the request,
excluding the charset definition.
*/
func (s *Server) RegisterCodec(codec Codec, contentType string) {
	s.codecs[strings.ToLower(contentType)] = codec
}

/*
RegisterService adds a new service to the server.

The name parameter is optional: if empty it will be inferred from
the receiver type name.

Methods from the receiver will be extracted if these rules are satisfied:

- The receiver is exported (begins with an upper case letter) or local
  (defined in the package registering the service).
- The method name is exported.
- The method has three arguments: *[Context Type], *args, *reply.
- All three arguments are pointers.
- The second and third arguments are exported or local.
- The method has return type error.

All other methods are ignored.
*/
func (s *Server) RegisterService(receiver interface{}, name string) error {
	return s.services.add(receiver, name, s.ctxType)
}

/*
HasMethod returns true if the given method is registered.

The method uses a dotted notation as in "Service.Method".
*/
func (s *Server) HasMethod(name string) bool {
	_, err := s.services.get(name);
	return err == nil
}

/*
return the map of names of services with its methods
*/
func (s *Server) ServiceMap() map[string][]string {
	return s.services.Map()
}

/*
ServeHTTP
 */
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		WriteError(w, 405, "rpc: POST method required, received "+r.Method)
		return
	}
	contentType := r.Header.Get("Content-Type")
	idx := strings.Index(contentType, ";")
	if idx != -1 {
		contentType = contentType[:idx]
	}
	var codec Codec
	if contentType == "" && len(s.codecs) == 1 {
		// If Content-Type is not set and only one codec has been registered,
		// then default to that codec.
		for _, c := range s.codecs {
			codec = c
		}
	} else if codec = s.codecs[strings.ToLower(contentType)]; codec == nil {
		WriteError(w, 415, "rpc: unrecognized Content-Type: "+contentType)
		return
	}

	// Create a new codec request.
	codecReq := codec.NewRequest(r)
	// Get service method to be called.
	method, errMethod := codecReq.Method()
	if errMethod != nil {
		codecReq.WriteError(w, 400, errMethod)
		return
	}

	methodSpec, errGet := s.services.get(method)
	if errGet != nil {
		codecReq.WriteError(w, 400, errGet)
		return
	}

	rValue := reflect.ValueOf(r)
	ctx := reflect.New(s.ctxType)

	// execute before functions before service call
	for _, fn := range s.beforeFns {
		if err := reflectFuncCall(fn, []reflect.Value{rValue, ctx,}); err != nil {
			codecReq.WriteError(w, 400, err)
			return
		}
	}

	// Decode the args.
	args := reflect.New(methodSpec.argsType)
	if errRead := codecReq.ReadRequest(args.Interface()); errRead != nil {
		codecReq.WriteError(w, 400, errRead)
		return
	}

	// create a new reply
	reply := reflect.New(methodSpec.replyType)

	// Call the service method.
	if err := reflectFuncCall(methodSpec.method.Func, []reflect.Value{
		methodSpec.service.rValue,
		ctx,
		args,
		reply,
	}); err != nil {
		codecReq.WriteError(w, 400, err)
		return
	}

	// execute after functions before service call
	for _, fn := range s.afterFns {
		if err := reflectFuncCall(fn, []reflect.Value{rValue, ctx,}); err != nil {
			codecReq.WriteError(w, 400, err)
			return
		}
	}

	w.Header().Set("x-content-type-options", "nosniff")
	codecReq.WriteResponse(w, reply.Interface())
}

/*
WriteError, a helper function to write error message to ResponseWriter
 */
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, msg)
}

/*
reflectFuncCall, a helper function to call a function in reflect way and return error
 */
func reflectFuncCall(fn reflect.Value, args []reflect.Value) error {
	errValue := fn.Call(args)
	var errResult error
	errInter := errValue[0].Interface()
	if errInter != nil {
		errResult = errInter.(error)
	}
	return errResult
}

/*
validCtxFunc validate context func
param fn shoule be type func(*http.Request, [Context Pointer Type]) error; and Context Pointer Type is of type param ctxType
*/
func validCtxFunc(fn interface{}, ctxType reflect.Type) error {
	if fn == nil {
		return fmt.Errorf("rpc: middleware is nil")
	}

	fnValue := reflect.ValueOf(fn)

	if fnValue.Type().Kind() != reflect.Func {
		return fmt.Errorf("rpc: middleware is not func type")
	}

	if fnValue.Type().NumIn() != 2 {
		return fmt.Errorf("rpc: middleware ill-fromed")
	}

	if fnValue.Type().NumOut() != 1 {
		return fmt.Errorf("rpc: middleware ill-fromed")
	}

	if inType := fnValue.Type().In(0); inType.Kind() != reflect.Ptr || inType.Elem() != reflect.TypeOf((*http.Request)(nil)).Elem() {
		return fmt.Errorf("rpc: middleware ill-fromed")
	}

	if inType := fnValue.Type().In(1); inType.Kind() != reflect.Ptr || inType.Elem() != ctxType {
		return fmt.Errorf("rpc: middleware ill-fromed")
	}

	if outType := fnValue.Type().Out(0); outType != reflect.TypeOf((*error)(nil)).Elem() {
		return fmt.Errorf("rpc: middleware ill-fromed")
	}

	return nil
}
