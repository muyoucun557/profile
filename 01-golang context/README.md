# context

context提供了用于在``across API boundaries and between processes``传递``截止日期deadlines``、``取消信号cancellation signals``、``请求域的值request-scoped value``。

不太理解的是``API boundaries and between processes``。从实际来看，是在函数之间、goroutine之间传递。

关于context在并发模式上的应用请参看这篇官方博客https://golang.google.cn/blog/context。
其主要意思是：在server处理请求的时候，针对DB的操作、缓存的操作、向其他服务发起请求等等，可能会启动多个goroutine来处理。当请求被取消或者超时的时候，应该立即释放资源。context在这个场景中提供了释放资源的能力。

## 