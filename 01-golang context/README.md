# context

context提供了在``across API boundaries and between processes``传递``截止日期deadlines``、``取消信号cancellation signals``、``请求域的值request-scoped value``的能力。

不太理解的是``API boundaries and between processes``。从实际来看，是在函数之间、goroutine之间传递。

关于context在并发模式上的应用请参看这篇官方博客https://golang.google.cn/blog/context。
其主要意思是：在server处理请求的时候，针对DB的操作、缓存的操作、向其他服务发起请求等等，可能会启动多个goroutine来处理。当请求被取消或者超时的时候，应该立即释放资源。context在这个场景中提供了释放资源的能力。

## API简介

1. WithValue，用于传递``请求域的值request-scoped value``
2. WithDeadline、WithTimeout，用于传递``截止日期deadlines``
3. WithCancel，用于传递``取消信号cancellation signals``
4. Background，生成一个empty context，没有``request-scoped value``、``deadlines``、``cancellation signals``，一般作为初始化的context，用在请求进来的时候。
5. TODO，和Background一样，生成一个empty context。当不确定是什么类型的context的时候就使用TODO。(我没有想到对应的场景)

## Context interface

context包定一个Context的interface，valueCtx、cancelerCtx、timerCtx均实现了该接口。
```Golang
type Context interface {
  // Deadline返回deadline。如果context不是deadline类型，则ok是false
	Deadline() (deadline time.Time, ok bool)

	// See https://blog.golang.org/pipelines for more examples of how to use
	// a Done channel for cancellation.
  // Done返回一个chan。如果ctx被取消了，那么Done返回的就是一个处于closed状态的chan。
  // 如果ctx没有被取消，那么返回的是一个未被close的chan。（并且在实现的时候，chan是lazy load的）
  // 有些ctx永远不会被取消，例如Background方法生成的ctx，这种情况下Done方法返回的是nil
	Done() <-chan struct{}

  // 如果ctx没有被取消，那么Err返回一个nil。如果ctx被取消，那么Err返回一个non-nil的error,err表示取消的原因。
  // 像Background返回的是一个emptyCtx，由于该ctx永远不会取消，那么返回的是一个nil
	Err() error

  // 返回key对应的Value。如果当前ctx找不到，那么在parentCtx中直到找到top-ctx。
  // 需要注意的是key必须具备campareable属性，在Value方法中判断是否相等用的是==。
  // 为了防止key出现类型碰撞，使用WithValue的时候应该为当前这次调用定义一个新的类型。
  // 例如:parent ctx中使用的key叫string("name")，当前ctx也需要使用叫name的key，此时会出现key的类型碰撞。
  // 为了解决这个问题，需要新定义一个类型，type StudentName string， key := StudentName("name")
	Value(key interface{}) interface{}
}
```

## emptyCtx






