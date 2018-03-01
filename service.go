// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2012 The Gorilla Authors. All rights reserved.
// Copyright 2018 Yi Jin. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type serviceMethod struct {
	service   *service       // pointer to parent service
	method    reflect.Method // receiver method
	argsType  reflect.Type   // type of the request argument
	replyType reflect.Type   // type of the response argument
}

type service struct {
	name    string                    // name of service
	methods map[string]*serviceMethod // registered methods
	rValue  reflect.Value             // receiver of methods for the service
}

// serviceMap is a registry for services.
type serviceMap struct {
	mutex    sync.Mutex
	services map[string]*service
}

/*
register adds a new service using reflection to extract its methods
*/
func (m *serviceMap) add(rcvr interface{}, name string, ctxType reflect.Type) error {
	if rcvr == nil {
		return fmt.Errorf("rpc: nil rcvr is not allowed")
	}

	s := &service{
		name:    name,
		rValue:  reflect.ValueOf(rcvr),
		methods: make(map[string]*serviceMethod),
	}

	// type name of rcvr as default name
	if len(s.name) == 0 {
		s.name = reflect.Indirect(s.rValue).Type().Name()
	}

	if s.name == "" {
		return fmt.Errorf("rpc: no service name for type %q", s.rValue.String())
	}

	// iterate methods
	for i := 0; i < s.rValue.Type().NumMethod(); i++ {
		m := s.rValue.Type().Method(i)

		if m.PkgPath != "" {
			continue
		}

		// Method needs four ins: receiver, ctx, *args, *reply.
		if m.Type.NumIn() != 4 {
			continue
		}

		// ctx
		if m.Type.In(1).Kind() != reflect.Ptr || m.Type.In(1).Elem() != ctxType {
			continue
		}

		// args
		args := m.Type.In(2)
		if args.Kind() != reflect.Ptr {
			continue
		}

		// reply
		reply := m.Type.In(3)
		if reply.Kind() != reflect.Ptr {
			continue
		}

		// error
		if m.Type.NumOut() != 1 {
			continue
		}

		if m.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}

		s.methods[m.Name] = &serviceMethod{
			service:   s,
			method:    m,
			argsType:  args.Elem(),
			replyType: reply.Elem(),
		}
	}

	if len(s.methods) == 0 {
		return fmt.Errorf("rpc: %q has no exported methods of suitable type", s.name)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.services == nil {
		m.services = make(map[string]*service)
	} else if _, ok := m.services[s.name]; ok {
		return fmt.Errorf("rpc: service %q already defined", s.name)
	}
	m.services[s.name] = s

	return nil
}

/*
get returns a registered service given a method name.
The method name uses a dotted notation as in "Service.Method".
*/
func (m *serviceMap) get(method string) (*serviceMethod, error) {
	parts := strings.Split(method, ".")
	if len(parts) != 2 {
		err := fmt.Errorf("rpc: service/method request ill-formed: %q", method)
		return nil, err
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	service := m.services[parts[0]]
	if service == nil {
		err := fmt.Errorf("rpc: can't find service %q", method)
		return nil, err
	}
	serviceMethod := service.methods[parts[1]]
	if serviceMethod == nil {
		err := fmt.Errorf("rpc: can't find method %q", method)
		return nil, err
	}
	return serviceMethod, nil
}

/*
return the map of names of services with its methods
*/
func (m *serviceMap) Map() (ret map[string][]string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ret = make(map[string][]string)
	for _, s := range m.services {
		ret[s.name] = make([]string, 0, len(s.methods))
		for method, _ := range s.methods {
			ret[s.name] = append(ret[s.name], method)
		}
	}
	return
}
