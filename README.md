# Golang Jsonrpc 2.0 Package

Modified from [Gorilla rpc & json2](https://github.com/gorilla/rpc/tree/master/v2)

- Add middlewares support.
- Use user-defined Context instead of http.Request

### Example

```go
package main

import (
	"net/http"
	"fmt"
	"net/http/httptest"
	"log"
	"bytes"
	"github.com/antenna3mt/rpc"
	"github.com/antenna3mt/rpc/json"
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
	reply.Text = args.Text + "!"
	return nil
}

func FetchAuthToken(r *http.Request, ctx *Context) error {
	ctx.AuthToken = r.Header.Get("Authorization")
	return nil
}

func Logger(r *http.Request, ctx *Context) error {
	log.Println("log test")
	return nil
}

func main() {
	server, err := rpc.NewServer(new(Context))
	if err != nil {
		log.Fatal(err)
	}
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterService(new(MyService), "")
	server.RegisterService(new(MyService), "Second")
	server.RegisterBeforeFunc(FetchAuthToken)
	server.RegisterAfterFunc(Logger)

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
		fmt.Println(reply.Text)
	}

}
```

