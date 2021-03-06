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

## Example

https://pkg.go.dev/context#example-WithValue

https://pkg.go.dev/context#example-WithCancel

https://pkg.go.dev/context#example-WithDeadline

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

// NOTE: 这个方法很重要。这个方法主要做两件事
// 1. 如果parent ctx已经被取消了，那么立马取消child（当前ctx）
// 2. 如果parent ctx没有被取消，那么维护一个关系，能让parent ctx在取消之后能立马取消child（当前ctx）
//    2.1 有两种方案来维护这个关系
//       -- 1. parent ctx中有一个map，map里存储的是它的所有的child ctx。parent ctx被取消的时候，取消map里的child
//       -- 2. 可以创建一个新的goroutine，在新goroutine中使用select监听parent ctx中的chan是否被关闭，如果被关闭那么直接取消child ctx
//       -- 3. 为什么要做出这样的设计，因为parent ctx可能是来自于外部包实现的context，外部包实现的context不一定存储child
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

// Done返回ctx的channel
// 且这里使用的是lazy load，这里使用了互斥锁用于保证并发安全
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

// Err返回context中的err，err表示ctx被取消的原因。有表示cancelCtx的取消，有表示timerCtx的取消（达到了deadline而取消）
// 当ctx被取消的时候，err会被赋值
// NOTE: 这里需要注意一下，当ctx被取消之后，其err会被赋值。可以通过err是不是等于nil来判断ctx是否被取消了
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

// cancel，取消context
func (c *cancelCtx) cancel(removeFromParent bool, err error) {
	if err == nil {
		panic("context: internal error: missing cancel error")
	}
	c.mu.Lock() // 为了保证并发安全，需要加锁
  // 1. 通过err是不是为nil来判断ctx有没有被取消过。如果已经取消了直接返回
	if c.err != nil {
		c.mu.Unlock()
		return // already canceled
	}
  // 2. lazy load channel。且关闭channel
	c.err = err
	d, _ := c.done.Load().(chan struct{})
	if d == nil {
		c.done.Store(closedchan)  // closedchan是一个全局变量，一个已经被close的channel
	} else {
		close(d)
	}
  // 3. 遍历child，调用child的cancel方法，取消child
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

// removeChild 将child从parent ctx的map中删除
// 这里也用到了parentCancelCtx方法用于判断（parent child之间的联系是通过goroutine还是通过map来维护的）
func removeChild(parent Context, child canceler) {
	p, ok := parentCancelCtx(parent)
	if !ok {
		return
	}
	p.mu.Lock()
	if p.children != nil {
		delete(p.children, child)
	}
	p.mu.Unlock()
}
```

## WithDeadline

``func WithDeadline(parent Context, d time.Time) (Context, CancelFunc)``
WithDeadline返回一个含有parent contetxt的child context，且deadline是d。如果parent context的deadline比d早，那么child context的deadline和parent context的deadline一样（对于这种场景child context是一个cancelCtx实例）。

带有deadline的context是通过timerCtx结构体来实现的。下面给出timerCtx的结构
```Golang
type timerCtx struct {
	cancelCtx
	timer *time.Timer // 一个定时器，达到deadline的时候会调用timerCtx的cancel函数

	deadline time.Time
}
```

## Background和TODO

这两个函数返回的是全局变量，参看下面代码
```Golang
var (
	background = new(emptyCtx)
	todo       = new(emptyCtx)
)
```
返回的都是emptyCtx对象。从语义上讲，这两个函数使用的场景不一样。
Background返回的empty context，没有request-scoped value、取消信号、截止日期这些数据。一般用在context链条的最顶端，例如server接收到一个请求之后，为这个请求生成一个empty context。
TODO返回的empty context，同样也是没有request-scoped value、取消信号、截止日期这些数据。一般不清楚使用哪个context的时候使用TODO。(真-不知道实际会有哪些场景)


## 机制和原理

### valueCtx

#### 一：能从context链条中向上查询value

valueCtx结构体源码
```Golang
type valueCtx struct {
	Context   // 通过匿名字段来实现继承
	key, val interface{}
}
```

WithValue如何生成一个valueCtx对象的
```Golang
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
  // Notice: 将Parent ctx作为匿名字段的值传到child ctx里
	return &valueCtx{parent, key, val}
}
```

valueCtx如何寻找对应的value
```Golang
func (c *valueCtx) Value(key interface{}) interface{} {
  // 如果当前ctx的key就是要寻找的key，返回当前ctx的value
	if c.key == key {
		return c.val
	}
  // 如果不是，调用parent ctx的方法，寻找key
	return c.Context.Value(key)
}
```

### cancelCtx

#### 一：调用cancel函数，能取消ctx

cancelCtx结构体中维护了一个chan，如果chan是关闭状态的chan，那么表示ctx是处于取消状态，如果没有不是关闭状态，那么表示ctx没有被取消。
同时cancelCtx维护了一个叫err的字段，当cancelCtx被取消的时候，err会被赋值。
cancelCtx提供了一个cancel函数。
```Golang
func (c *cancelCtx) cancel(removeFromParent bool, err error) {
  // 1. 取消ctx必须得有一个“原因”
	if err == nil {
		panic("context: internal error: missing cancel error")
	}
  // 2. 为了并发安全，这里加了互斥锁
  // 在关闭chan的时候，将error赋值给了ctx.err，可以通过ctx.err是否为nil来判断ctx是否被取消了
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return // already canceled
	}
	c.err = err
  // 3. 这里的chan使用的是lazy load chan，切统一使用一个全局变量closedchan
  // 也可以生成一个新的chan，但是这样会浪费资源（使用closedchan，避免频繁的创建一个chan，节省开销）。
	d, _ := c.done.Load().(chan struct{})
	if d == nil {
		c.done.Store(closedchan)
	} else {
		close(d)
	}
  // 4. 取消child ctx
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
```

#### 二：如果parent ctx被取消了，那么child ctx也会被取消

在cancelCtx中维护了parent ctx与child ctx之间的联系，只有保持了这样的联系，才能在parent ctx被取消的时候也会取消child ctx。
如何去维护这种联系的呢？通过两种方式去联系的。
第一种：在parent ctx中维护一个map，用map存储所有的child ctx。
第二种：新建一个goroutine，在这个goroutine中监视parent ctx的chan被关闭的事件。
```Golang
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

  // Notice: 通过parentCancelCtx函数来判断使用那种方式维护parent ctx与child ctx。
	if p, ok := parentCancelCtx(parent); ok {
		p.mu.Lock()
		if p.err != nil {
			// parent has already been canceled
			child.cancel(false, p.err)
		} else {
			if p.children == nil {
				p.children = make(map[canceler]struct{})
			}
      // NOTICE: 在parent ctx中维护一个map，用map存储所有的child ctx。
      // 在parent ctx调用cancel方法的时候，回调用child ctx的cancel方法
			p.children[child] = struct{}{}
		}
		p.mu.Unlock()
	} else {
		atomic.AddInt32(&goroutines, +1)
    // NOTICE: 新建一个goroutine，在这个goroutine中监视parent ctx的chan被关闭的事件。
		go func() {
			select {
			case <-parent.Done():
				child.cancel(false, parent.Err())
			case <-child.Done():
			}
		}()
	}
}
```

### timerCtx

timerCtx需要指定一个parent ctx与deadline。在cancel函数被调用、或者parent ctx的cancel会调用、或者时间到了deadline，timerCtx就会被取消。

timerCtx的结构如下
```Golang
type timerCtx struct {
	cancelCtx
	timer *time.Timer // Under cancelCtx.mu.

	deadline time.Time
}
```
继承了cancelCtx，借助cancelCtx来实现了一但parent ctx被取消，那么child ctx也会被取消。
同时还存在一个定时器，用于在时间到达deadline的时候，调用cancel方法。
如果child ctx的deadline比parent ctx的deadline要晚，那么就生成child ctx的类型是cancelCtx。此时是不需要timer的，因此无需生成timerCtx类型。下面给出代码
```Golang
func WithDeadline(parent Context, d time.Time) (Context, CancelFunc) {
  ...
	if cur, ok := parent.Deadline(); ok && cur.Before(d) {
		// The current deadline is already sooner than the new one.
		return WithCancel(parent)
	}
	...
}
```
如果生成的是cancelCtx，那么获取的parent ctx的deadline。cancelCtx的parent ctx是timerCtx。


## Go并发模式下的Context
可看这篇博客 https://golang.google.cn/blog/context