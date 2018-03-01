package test

import (
	"bytes"
	"fmt"
	"github.com/antenna3mt/rpc"
	"github.com/antenna3mt/rpc/json"
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

const MyToken = "MyToken"

type Context struct {
	AuthToken string
}

type MyService struct{}

func (*MyService) Hello(ctx *Context, args *struct {
	Text string
}, reply *struct {
	Text string
}) error {
	if ctx.AuthToken != MyToken {
		return fmt.Errorf("authorization fail")
	}
	reply.Text = args.Text
	return nil
}

func FetchAuthToken(r *http.Request, ctx *Context) error {
	ctx.AuthToken = r.Header.Get("Authorization")
	return nil
}

func Logger(r *http.Request, ctx *Context) error {
	fmt.Println("log test")
	return nil
}

func TestServer(t *testing.T) {
	server, err := rpc.NewServer(new(Context))
	if err != nil {
		log.Fatal(err)
	}
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterService(new(MyService), "")
	server.RegisterService(new(MyService), "Second")
	server.RegisterBeforeFunc(FetchAuthToken)
	server.RegisterAfterFunc(Logger)

	func() {
		reqBody, _ := json.EncodeClientRequest("MyService.Hello", &struct{ Test string }{"Hello Rpc"})
		req := httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBody))
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		resp := w.Result()
		reply := &struct{ Text string }{}
		assert.Error(t, json.DecodeClientResponse(resp.Body, reply))
	}()

	func() {
		reqBody, _ := json.EncodeClientRequest("MyService.Hello", &struct{ Text string }{"Hello Rpc"})
		req := httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", MyToken)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		resp := w.Result()
		reply := &struct{ Text string }{}
		if err := json.DecodeClientResponse(resp.Body, reply); err != nil {
			log.Fatal(err)
		} else {
			assert.Equal(t, "Hello Rpc", reply.Text)
		}

	}()
}
