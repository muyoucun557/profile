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
