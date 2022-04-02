# context

context提供了在``across API boundaries and between processes``传递``截止日期deadlines``、``取消信号cancellation signals``、``请求域的值request-scoped value``的能力。

不太理解的是``API boundaries and between processes``。从实际来看，是在函数之间、goroutine之间传递。

关于context在并发模式上的应用请参看这篇官方博客https://golang.google.cn/blog/context。
其主要意思是：在server处理请求的时候，针对DB的操作、缓存的操作、向其他服务发起请求等等，可能会启动多个goroutine来处理。当请求被取消或者超时的时候，应该立即释放资源。context在这个场景中提供了释放资源的能力。

## API简介

1. WithValue，用于传递``请求域的值request-scoped value``
2. WithDeadline、WithTimeout，用于传递``截止日期deadlines``
3. WithCancel，用于传递``取消信号cancellation signals``
4. Background，生成一个empty context，没有``request-scoped value``、``deadlines``、``cancellation signals``，一般作为初始化的context，用在请求进来的时候
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

emptyCtx是一个从不cancel、没有value、没有deadline的ctx。emptyCtx不对外暴露。只通过Background和TODO两个方法对外暴露一个emptyCtx的对象。

```Golang
type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (*emptyCtx) Done() <-chan struct{} {
	return nil
}

func (*emptyCtx) Err() error {
	return nil
}

func (*emptyCtx) Value(key interface{}) interface{} {
	return nil
}

func (e *emptyCtx) String() string {
	switch e {
	case background:
		return "context.Background"
	case todo:
		return "context.TODO"
	}
	return "unknown empty Context"
}
```

## WithValue

用于生成一个valueCtx。valueCtx存储一对(k,v)，用于传递request-scoped value。
```Golang

type valueCtx struct {
	Context
	key, val interface{}
}

func (c *valueCtx) String() string {
	return contextName(c.Context) + ".WithValue(type " +
		reflectlite.TypeOf(c.key).String() +
		", val " + stringify(c.val) + ")"
}

func (c *valueCtx) Value(key interface{}) interface{} {
	if c.key == key {
		return c.val
	}
	return c.Context.Value(key)
}

// 为了防止key出现类型碰撞，使用WithValue的时候应该为当前这次调用定义一个新的类型。
// 例如:parent ctx中使用的key叫string("name")，当前ctx也需要使用叫name的key，此时会出现key的类型碰撞。
// 为了解决这个问题，需要新定义一个类型，type StudentName string， key := StudentName("name")
func WithValue(parent Context, key, val interface{}) Context {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	if key == nil {
		panic("nil key")
	}
	if !reflectlite.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}
	return &valueCtx{parent, key, val}
}
```

## WithCancle

用于携带取消信号

```Golang

// 取消函数的类型。取消函数调用之后，表示该ctx会进入canceled状态。该函数会被多个goroutine同时调用，
// 一旦被调用，随后的调用什么都不会处理（借助了互斥锁）
type CancelFunc func()

// cancelCtx携带cancel信号的ctx。
// parent ctx被取消的时候，其children ctx也会被取消
// 调用cancel函数，会取消当前的ctx。每个cancelCtx都会对应一个cancel函数
// 如果parent永远不会被取消，那么child只能在cancel函数被调用的时候才会被取消
type cancelCtx struct {
	Context

	mu       sync.Mutex            // protects following fields
	done     atomic.Value          // of chan struct{}, created lazily, closed by first cancel call
	children map[canceler]struct{} // set to nil by the first cancel call
	err      error                 // set to non-nil by the first cancel call
}

func WithCancel(parent Context) (ctx Context, cancel CancelFunc) {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
  // 基于parent ctx创建一个新的cancel ctx
	c := newCancelCtx(parent)
	propagateCancel(parent, &c)
	return &c, func() { c.cancel(true, Canceled) }
}

// newCancelCtx returns an initialized cancelCtx.
func newCancelCtx(parent Context) cancelCtx {
	return cancelCtx{Context: parent}
}

// FIXME: 这个方法很重要。这个方法主要做两件事
// 1. 如果parent ctx已经被取消了，那么立马取消child（当前ctx）
// 2. 如果parent ctx没有被取消，那么维护一个关系，能让parent ctx在取消之后能立马取消child（当前ctx）
//    2.1 有两种方案来维护这个关系
//       -- 1. parent ctx中有一个map，map里存储的是它的所有的child ctx。parent ctx被取消的时候，取消map里的chold
//       -- 2. 可以创建一个新的goroutine，在新goroutine中使用select监听parent ctx中的chan是否被关闭，如果被关闭那么直接取消child ctx
// 3. 针对2种的问题，parentCancelCtx方法就是用来确定用哪个方案来维护这个关系的
func propagateCancel(parent Context, child canceler) {
	done := parent.Done()
	if done == nil {
		return // parent is never canceled
	}

	select {
	case <-done:
		// parent is already canceled
		child.cancel(false, parent.Err())
		return
	default:
	}

	if p, ok := parentCancelCtx(parent); ok {
		p.mu.Lock()
		if p.err != nil {
			// parent has already been canceled
			child.cancel(false, p.err)
		} else {
			if p.children == nil {
				p.children = make(map[canceler]struct{})
			}
			p.children[child] = struct{}{}
		}
		p.mu.Unlock()
	} else {
		atomic.AddInt32(&goroutines, +1)
		go func() {
			select {
			case <-parent.Done():
				child.cancel(false, parent.Err())
			case <-child.Done():
			}
		}()
	}
}


func (c *cancelCtx) Value(key interface{}) interface{} {
	if key == &cancelCtxKey {
		return c
	}
	return c.Context.Value(key)
}

func (c *cancelCtx) Done() <-chan struct{} {
	d := c.done.Load()
	if d != nil {
		return d.(chan struct{})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d = c.done.Load()
	if d == nil {
		d = make(chan struct{})
		c.done.Store(d)
	}
	return d.(chan struct{})
}

func (c *cancelCtx) Err() error {
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	return err
}

type stringer interface {
	String() string
}

func contextName(c Context) string {
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return reflectlite.TypeOf(c).String()
}

func (c *cancelCtx) String() string {
	return contextName(c.Context) + ".WithCancel"
}

// cancel closes c.done, cancels each of c's children, and, if
// removeFromParent is true, removes c from its parent's children.
func (c *cancelCtx) cancel(removeFromParent bool, err error) {
	if err == nil {
		panic("context: internal error: missing cancel error")
	}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return // already canceled
	}
	c.err = err
	d, _ := c.done.Load().(chan struct{})
	if d == nil {
		c.done.Store(closedchan)
	} else {
		close(d)
	}
	for child := range c.children {
		// NOTE: acquiring the child's lock while holding parent's lock.
		child.cancel(false, err)
	}
	c.children = nil
	c.mu.Unlock()

	if removeFromParent {
		removeChild(c.Context, c)
	}
}

// parentCancelCtx
// 1. 如果parent ctx已经被关闭了或者从来不会被关闭，返回nil和false
// 2. parent.Value在parent所在的ctx链条中，判断是不是存在一个cancelCtx变量，如果不存在返回nil和false
// 3. 如果存在判断parent和cancelCtx变量是不是同一个，是同一个返回cancelCtx变量指针和true；不是同一个，返回nil和false
func parentCancelCtx(parent Context) (*cancelCtx, bool) {
	done := parent.Done()
	if done == closedchan || done == nil {
		return nil, false
	}
	p, ok := parent.Value(&cancelCtxKey).(*cancelCtx)
	if !ok {
		return nil, false
	}
	pdone, _ := p.done.Load().(chan struct{})
	if pdone != done {
		return nil, false
	}
	return p, true
}
```
