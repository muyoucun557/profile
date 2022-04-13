# Go的并发模式：context

是golang官方博客的翻译，可参看<https://go.dev/blog/context>


## Introduction
在Go的服务中，对于每一个请求都会有一个goroutine进行处理。```Request Handler```通常开启额外的goroutine去访问后端，比如访问database和发起RPC调用。这些额外开启的goroutine回去访问一些特定的值，比如：用户的id，authorization tokens，和请求的deadline（不太明白为什么会有request's deadline）。当一个请求被取消或者超时，所有的工作在这个请求上的goroutine都应该快速退出，为了回收所有正在使用中的资源。

在Google，我们开发了``context``库让在goroutine之间传递``request-scoped value``、``取消信号``、``截止日期``变得很方便。这篇文章描述了怎样去使用这个库，并且提供了一个完整的可工作的实例。

## Context
``context``的核心库是``Context``类型：
```Go
// A Context carries a deadline, cancellation signal, and request-scoped values
// across API boundaries. Its methods are safe for simultaneous use by multiple
// goroutines.
type Context interface {
    // Done returns a channel that is closed when this Context is canceled
    // or times out.
    Done() <-chan struct{}

    // Err indicates why this context was canceled, after the Done channel
    // is closed.
    Err() error

    // Deadline returns the time when this Context will be canceled, if any.
    Deadline() (deadline time.Time, ok bool)

    // Value returns the value associated with key or nil if none.
    Value(key interface{}) interface{}
}
```
``Done``方法返回了一个channel用来充当取消信号。当channel被关闭的时候，函数应该放弃执行并且返回。``Err``方法返回一个error表明为什么被取消。[Pipelines and Cancellation](https://go.dev/blog/pipelines)文章对``channel``讨论的更多。

``Context``没有``Cancel``方法是因为``Done channel``是只能接收的，接收取消信号的函数通常不会是发送信号的。尤其，当一个父操作开启一个goroutine进行子操作的时候，子操作不能够取消父操作。``WithCancel``函数提供了一个途径用于取消一个新的``Context``值。

``Context``是并发安全的。在编写代码的时候，一个``Context``能够传给任意数量的goroutine，并且都能够取消他们。

``Deadline``方法允许函数去决定它们是否应该开始工作。如果时间太少，那是不值得的。通常使用deadline给I/O操作设置timeout。

``Value``允许一个context携带``request-scoped``数据。携带的数据必须是并发安全的。

## 例子：Google Web Search

```Go
func handleSearch(w http.ResponseWriter, req *http.Request) {
    // ctx is the Context for this handler. Calling cancel closes the
    // ctx.Done channel, which is the cancellation signal for requests
    // started by this handler.
    var (
        ctx    context.Context
        cancel context.CancelFunc
    )
    timeout, err := time.ParseDuration(req.FormValue("timeout"))
    if err == nil {
        // The request has a timeout, so create a context that is
        // canceled automatically when the timeout expires.
        ctx, cancel = context.WithTimeout(context.Background(), timeout)
    } else {
        ctx, cancel = context.WithCancel(context.Background())
    }
    defer cancel() // Cancel ctx as soon as handleSearch returns.

    query := req.FormValue("q")
    if query == "" {
        http.Error(w, "no query", http.StatusBadRequest)
        return
    }
    userIP, err := userip.FromRequest(req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    ctx = userip.NewContext(ctx, userIP)

    // Run the Google search and print the results.
    start := time.Now()
    results, err := google.Search(ctx, query)
    elapsed := time.Since(start)

    if err := resultsTemplate.Execute(w, struct {
        Results          google.Results
        Timeout, Elapsed time.Duration
    }{
        Results: results,
        Timeout: timeout,
        Elapsed: elapsed,
    }); err != nil {
        log.Print(err)
        return
    }
}
```
```Go
func Search(ctx context.Context, query string) (Results, error) {
    // Prepare the Google Search API request.
    req, err := http.NewRequest("GET", "https://ajax.googleapis.com/ajax/services/search/web?v=1.0", nil)
    if err != nil {
        return nil, err
    }
    q := req.URL.Query()
    q.Set("q", query)

    // If ctx is carrying the user IP address, forward it to the server.
    // Google APIs use the user IP to distinguish server-initiated requests
    // from end-user requests.
    if userIP, ok := userip.FromContext(ctx); ok {
        q.Set("userip", userIP.String())
    }
    req.URL.RawQuery = q.Encode()

    var results Results
    err = httpDo(ctx, req, func(resp *http.Response, err error) error {
        if err != nil {
            return err
        }
        defer resp.Body.Close()

        // Parse the JSON search result.
        // https://developers.google.com/web-search/docs/#fonje
        var data struct {
            ResponseData struct {
                Results []struct {
                    TitleNoFormatting string
                    URL               string
                }
            }
        }
        if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
            return err
        }
        for _, res := range data.ResponseData.Results {
            results = append(results, Result{Title: res.TitleNoFormatting, URL: res.URL})
        }
        return nil
    })
    // httpDo waits for the closure we provided to return, so it's safe to
    // read results here.
    return results, err
}
```
```Go
func httpDo(ctx context.Context, req *http.Request, f func(*http.Response, error) error) error {
    // Run the HTTP request in a goroutine and pass the response to f.
    c := make(chan error, 1)
    req = req.WithContext(ctx)
    go func() { c <- f(http.DefaultClient.Do(req)) }()
    select {
    case <-ctx.Done():
        // <-c // Wait for f to return. // 假设这一行注释掉
        return ctx.Err()
    case err := <-c:
        return err
    }
}
```

问题：httpDo函数中，如果ctx的done channel是先被关闭，那么对于执行函数f的goroutine是不会结束的。因此这个goroutine是无法回收的，对于Introduction中介绍的回收所有gorotuine有出入。

## 为Context适配代码

许多server框架提供了库和类型用于携带``request-scoped``value。我们可以给``context 接口``定义一个新的实现去桥接server框架和预期使用``Context``作为参数的代码。

For example, Gorilla’s github.com/gorilla/context package allows handlers to associate data with incoming requests by providing a mapping from HTTP requests to key-value pairs. In gorilla.go, we provide a Context implementation whose Value method returns the values associated with a specific HTTP request in the Gorilla package.

Other packages have provided cancellation support similar to Context. For example, Tomb provides a Kill method that signals cancellation by closing a Dying channel. Tomb also provides methods to wait for those goroutines to exit, similar to sync.WaitGroup. In tomb.go, we provide a Context implementation that is canceled when either its parent Context is canceled or a provided Tomb is killed.

## 总结

在Google，我们要求Go程序员给每个发起请求的函数都传递一个``context``参数作为出参和入参的第一个参数。这能够让在不同团队中的开发的Go代码interoperate well。

## PS
翻译的不好，这篇文章主要的意思是：在并发模式下，context用于回收资源。
