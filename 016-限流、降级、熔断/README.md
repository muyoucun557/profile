# 限流、降级、熔断

## 限流

在请求多的情况下，限制一部分请求，从而保证系统的稳定性。

### 常见的限流算法

#### 计数限流

假设系统能同时处理100个请求，用一个计数器，接收到一个请求就加1，处理完一个请求就减1。每次来请求的时候，判断一下计数器是否超过阈值。如果超过，则拒绝。

如果计数器在内存中，那么就是单机限流算法。如果是计数器在第三方存储里，例如redis，集群机器访问就是分布式限流算法。

优点：简单
缺点：假设阈值是1w，当前计数器是0，1w个请求同时涌进来，这种突发流量是挡不住的。

#### 固定窗口限流

在计数器的基础上增加一个固定时间窗口逻辑。计数器每过一个时间窗口就重置。
固定窗口会存在窗口临界问题。

#### 滑动窗口限流

解决固定窗口临界问题。

#### 漏桶算法

桶的总容量大小不变，按照固定速度处理请求。当桶中请求的数量达到最大值，那么请求会被拒绝。

缺点：面对突发流量的时候，我们希望在保证系统稳定的情况下，能快速的处理请求。该算法是固定的速度，因此不能解决该问题

#### 令牌桶算法

匀速往桶中放入令牌，桶中令牌总数有限。请求来了向桶中获取令牌，获取成功，则处理，反之则拒绝。
假设桶中有100个令牌，在一瞬间被拿走，也就意味着能处理突发流量。

### 单机限流和分布式限流

单机限流和分布式限流实现的区别在于计数器的存储位置，如果存储在redis作为公用计数器（并非一定是redis，是公用存储即可，redis的性能比较高，一般选择redis），那么就是分布式限流算法，如果存储在内存中，每个服务一个，那么就是单机限流。

#### 滑动窗口的分布式限流算法实现

1. 假设限流的周期是1000ms，当前时间是ts(ms时间戳)
2. 借助sorted set来时间，往sorted set中存储一条数据，score是ts，member也可以是ts
3. 删除score在[0, ts-1000]范围内的数据
4. 统计当前sorted set内元素的数量，判断数量是否大于阈值，如果大于那么拒绝请求。
5. 给sorted set这是一个过期时间（节省内存）

下面给出实现的代码
```Go
func TestDisSlidingWindow(t *testing.T) {
	// 统计周期是1000ms，阈值是1000，请求的接口是"/user/:id"
	const threshold = 1000
	const window = 1000
	const source = "/user/:id"
	c := redis.NewClient(&redis.Options{
		Addr: "192.168.122.128:6379",
		DB:   0, // use default DB
	})

	// 用pipe，节省开销，结果一起返回
	pip := c.Pipeline()
	ts := time.Now().UnixMilli()
	ctx := context.Background()

	// 增加
	pip.ZAdd(ctx, source, &redis.Z{
		Score:  float64(ts),
		Member: ts,
	})
	// 删除
	pip.ZRemRangeByScore(ctx, source, fmt.Sprintf("%d", 0), fmt.Sprintf("%d", ts-window))
	// 统计数量
	count := pip.ZCard(ctx, source)
	// 设置过期时间
	pip.Expire(ctx, source, 2*window*time.Millisecond)
	_, err := pip.Exec(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	num, err := count.Uint64()
	if err != nil {
		log.Fatalln(err)
	}
	if num > threshold {
		fmt.Println("out of bound, reject")
		return
	}
	fmt.Println("will handle req")
}
```
NOTICE:这里使用的是pipeline。这里使用pipeline的好处是能节省RTT，可参看<http://redis.cn/topics/pipelining.html>。

<strong>问题：pipeline会不会有问题？</strong>
会存在精度丢失问题。影响不大。
<strong>使用tx是不是更好一些？</strong>
会更好一些，但是需要对比一下二者的性能。

下面给出性能对比：
环境：本机虚拟机，centos7，redis和客户端在一个机器里。循环执行1w次，对比时间。
经反复验证：pipiline执行时间大约在7s，tx执行时间大约在8s。性能有些偏差，但是不大。
这里面没贴出机器配置，原因只是为了针对这两种方案做对比，无需机器详细配置。

<strong>问题：按毫秒时间戳来处理，精度是否足够？</strong>
在我们公司有很多接口要求达到7000的qps，而1s只有1000ms。这样精度是不够的，解决这个问题可以使用微秒时间戳。

<strong>问题：在大流量场景下，性能是否有瓶颈？</strong>
redis的大约每秒能处理10多w的读写。我统计过我们公司的系统高峰时期，每秒的请求请求达到了30w以上，并且我们限流操作的时候会有5个操作，这样极大的放大了redis的负载。该怎么解决？
有两种方案：

1. 使用redis-cluster
2. 批量思想：上面的流程来看，每执行一次操作其实就是获取一个分额，如果每次的操作表示获取多个分额，那么能极大的减少对redis的访问。下面给出具体的流程。

假设在ts时间执行了zadd，表示获取了10个分额，计数器的值是10。
在接到请求的时候，判断当前时间与ts在不在一个时间窗口内，如果在，计数器减1，执行请求。如果不在，重新获取分额。
如果计数器是0，那么需要重新获取。

#### 如何基于redis做漏桶、令牌桶的分布式限流算法

### 限流的难点

每个限流都有一个阈值，如何设置这个阈值会比较难。定大了服务器会扛不住，定小了就误杀了。
我们怎么做的？
我们公司使用的是阿里云的SLB，SLB提供了流量指标（流入流量+丢弃流量）。拿业务爆发时期的高峰流量与以前业务平稳时期的高峰流量进行对比，得到一个比值。再统计出业务平稳时期的qps和tps，相乘即可得到现在的qps和tps。这就是阈值。这种算法，遇到动态扩容的场景就GG了，我们可以预留一些资源，将阈值调大。

阿里的sentinel借鉴了TCP的BBR算法，实现了一套基于负载的限流。