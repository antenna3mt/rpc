package rpc

import (
	"github.com/stretchr/testify/assert"
	"log"
	"reflect"
	"testing"
)

type Context struct{}

type TestService struct{}

func (*TestService) Hello(ctx *Context, args *struct{}, reply *struct{}) error {
	return nil
}

func TestServiceMap(t *testing.T) {
	services := new(serviceMap)

	if err := services.add(new(TestService), "", reflect.TypeOf(Context{})); err != nil {
		log.Fatal(err)
	}

	if method, err := services.get("TestService.Hello"); err != nil {
		log.Fatal(err.Error())
	} else {
		assert.Equal(t,
			method.service.methods["Hello"],
			method)
	}

	if err := services.add(new(TestService), "Second", reflect.TypeOf(Context{})); err != nil {
		log.Fatal(err)
	}

	if method, err := services.get("Second.Hello"); err != nil {
		log.Fatal(err)
	} else {
		assert.Equal(t,
			method.service.methods["Hello"],
			method)
	}

	if err := services.add(new(TestService), "", reflect.TypeOf(Context{})); err == nil {
		assert.Fail(t, "duplicate error")
	}

	m := services.Map()
	assert.Equal(t, map[string][]string{
		"TestService": []string{"Hello"},
		"Second":      []string{"Hello"},
	}, m)
}
