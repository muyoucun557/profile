# Nacos

Nacos的官网<https://nacos.io/zh-cn/>。
官方定义是：一个更易于构建云原生应用的动态服务发现、配置管理和服务管理平台。
这里的形容词是``更易于构建云原生应用的``,目前不知道它具备哪些特性能够更容易构建云原生应用。
它具备的功能
1. 动态服务配置
2. 服务发现及管理
3. 动态DNS服务
4. 服务及其元数据管理

## Nacos部署
Nacos是一个内部组件，并非面向外网环境的产品。支持三种部署模式
1. 单机模式
2. 集群模式
3. 多集群模式

### 存储依赖

0.7版本之前使用的是嵌入式数据库实现数据存储，不方便数据观察。0.7版本增加了支持mysql的数据源的能力。
1. mysql版本要求，5.6.5以上
2. 初始化mysql数据库，需要执行<https://github.com/alibaba/nacos/blob/develop/distribution/conf/nacos-mysql.sql>
3. 修改/conf/application.properties文件,格式如下
```properties
spring.datasource.platform=mysql

db.num=1
db.url.0=jdbc:mysql://11.162.196.16:3306/nacos_devtest?characterEncoding=utf8&connectTimeout=1000&socketTimeout=3000&autoReconnect=true
db.user=nacos_devtest
db.password=youdontknow
```

### 基于docker部署-单机模式
<https://hub.docker.com/r/nacos/nacos-server>中介绍了怎么部署。在文章的尾部介绍了怎么在docker环境下怎么配置。

```text
If the above property configuration list does not meet your requirements, you can mount the custom.properties file into the /home/nacos/init.d/ 
directory of the container, where the spring properties can be configured, and the priority is higher than application.properties file
```
意思是：将宿主机中的配置文件custom.properties挂载到container的/home/nacos/init.d/文件夹下，nacos启动的时候会加载配置文件。

下面给出custom.properties的配置
```text 
server.contextPath=/nacos
server.servlet.contextPath=/nacos
server.port=8848

spring.datasource.platform=mysql
db.num=1
db.url.0=jdbc:mysql://mysql-host:3306/nacos
db.user=user
db.password=pwd

nacos.cmdb.dumpTaskInterval=3600
nacos.cmdb.eventTaskInterval=10
nacos.cmdb.labelTaskInterval=300
nacos.cmdb.loadDataAtStart=false
management.metrics.export.elastic.enabled=false
management.metrics.export.influx.enabled=false
server.tomcat.accesslog.enabled=true
server.tomcat.accesslog.pattern=%h %l %u %t “%r” %s %b %D %{User-Agent}i
nacos.security.ignore.urls=/,//*.css,//.js,/**/.html,//*.map,//.svg,/**/.png,//*.ico,/console-fe/public/,/v1/auth/login,/v1/console/health/,/v1/cs/,/v1/ns/,/v1/cmdb/,/actuator/,/v1/console/server/
nacos.naming.distro.taskDispatchThreadCount=1
nacos.naming.distro.taskDispatchPeriod=200
nacos.naming.distro.batchSyncKeyCount=1000
nacos.naming.distro.initDataRatio=0.9
nacos.naming.distro.syncRetryDelay=5000
nacos.naming.data.warmup=true
nacos.naming.expireInstance=true
```

启动容器
```
docker run --name nacos -d -p 8848:8848 -e MODE=standalone -e PREFER_HOST_MODE=hostname -v /opt/docker_volumn/nacos/logs:/home/nacos/logs -v /opt/docker_volumn/nacos/custom.properties:/home/nacos/init.d/custom.properties nacos/nacos-server
```
为了方便观看log，将宿主机的``/opt/docker_volumn/nacos/logs``挂载到了container的``/home/nacos/logs``。这样就能在宿主机中查看log了。
容器启动之后，发现nacos尚未启动，经排查，nacos启动会写入日志，出现写入权限的问题。

排查的过程
1. 宿主机的``/opt/docker_volumn/nacos/logs``挂载到了container的``/home/nacos/logs``，``/home/nacos/logs``文件夹是nacos写入日志的路径。在宿主机中查看日志，发现无任何日志文件
2. 进入容器，执行``/home/nacos/bin/docker-startup.sh``，在容器中启动nacos，控制台打印出日志，经查看错误日志发现，是在``/home/nacos/logs``文件夹中创建文件失败，失败原因是权限问题

解决方案
原因是container中的root只是一个普通用户，不具备真正的root权限。右边文件给出了3中解决方案<https://blog.csdn.net/u012326462/article/details/81038446>

## Nacos的一般使用

### 配置命名空间

可以创建新的命名空间，对命名空间进行增删改查。

### KV配置

选择某个命名空间，创建KV，在nacos中，这里的K由data id和group组合在一起，具备唯一性。
V的值可以有多种类型，text、json、xml、yaml、html、properites。

### 服务配置

服务配置具备下面属性
1. 服务名称
2. 保护阈值：设置一个0~1之间的值，表示健康服务实例数量/当前服务总实例数量。正常情况下，nacos会将健康的实例给客户端，当比例小于阈值时会触发保护机制，将所有的实例（包括不健康的实例）一起返回给客户端。这样做的目的是： 防止在健康实例很少的时候出现大流量，将健康实例打垮（不健康实例返回给客户端，流量会有一部分分到不健康实例中）。
3. 分组，服务名和分组组合在一起在同一个namespace下具备唯一性
4. 元数据
5. 服务路由类型：设置服务的路由策略，默认是none。如果设置成label，需要设置响应的表达式来匹配实例，从而完成自定义的负载均衡

### 临时和永久

注册节点的时候，会有临时节点和永久节点之分。体现在注册接口中的参数Ephemeral。但是我发现对于service也存在临时service和永久service。下面给出三种情况

#### nacos中service不存在，直接调用注册接口，注册一个临时节点

此时会在nacos中自动创建一个service，且该service不允许注册永久节点。如果注册一个永久节点会抛出如下报错
``caused: errCode: 400, errMsg: Current service DEFAULT_GROUP@@user-svc is ephemeral service, can't register persistent instance. ;``
报错的大概意思是：service是一个临时service，不允许注册一个永久节点

#### nacos中service不存在，直接调用注册接口，注册一个永久节点

此时会在nacos中自动创建一个service，且该service不允许注册临时节点。如果注册一个临时节点会抛出如下报错
``caused: errCode: 400, errMsg: Current service DEFAULT_GROUP@@user-svc is persistent service, can't register ephemeral instance. ;``
报错的大概意思是：service是一个永久service，不允许注册一个临时节点

#### 通过nacos的控制台创建一个service，然后分别注册临时和永久的节点

NOTICE:在nacos控制台中创建servie的时候，并不能配置service的临时属性和永久属性。
经实践验证，无法注册临时节点。

TODO: service的临时永久属性。

### 服务注册与发现

发现包含查询和订阅。
问题：希望能通过nacos实现服务注册与发现，且能支持按照实例的负载进行负载均衡。
```Go
func TestRegisterAndDiscovery(t *testing.T) {
	// 1. 注册两个实例，定时上报负载
	for i := 0; i < 5; i++ {
		go func(index int) {
			cc := constant.ClientConfig{
				NamespaceId: "eeaec337-1f55-43ac-89a4-a5f96ab62c05", // 我自己创建的一个namespace
				Endpoint:    "192.168.122.128",
				Username:    "",
				Password:    "",
			}

			sc := []constant.ServerConfig{
				{
					IpAddr: "192.168.122.128",
					Port:   8848,
				},
			}
			client, err := clients.NewNamingClient(vo.NacosClientParam{
				ClientConfig:  &cc,
				ServerConfigs: sc,
			})
			if err != nil {
				panic(err)
			}
			ip := fmt.Sprintf("192.168.122.128.%d", index)
			de := false
			go func() {
				time.Sleep(time.Duration(30+index) * time.Second)
				client.DeregisterInstance(vo.DeregisterInstanceParam{
					Ip:          ip,
					Port:        8000,
					ServiceName: "user",
				})
				de = true
			}()
			for {
				loadNum := rand.Int31()
				load := fmt.Sprintf("%d", loadNum)
				b, err := client.RegisterInstance(vo.RegisterInstanceParam{
					Ip:          ip,
					Port:        8000,
					Weight:      10,
					Enable:      true,
					Healthy:     true,
					Metadata:    map[string]string{"load": load},
					ServiceName: "user",
					Ephemeral:   false,
				})
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(b)
				time.Sleep(3 * time.Second)
				if de {
					return
				}
			}
		}(i)
	}
	// 2. 开启一个发现goroutine，除了发现，还需要订阅

	go func() {
		cc := constant.ClientConfig{
			NamespaceId: "eeaec337-1f55-43ac-89a4-a5f96ab62c05", // 我自己创建的一个namespace
			Endpoint:    "192.168.122.128",
			Username:    "",
			Password:    "",
		}

		sc := []constant.ServerConfig{
			{
				IpAddr: "192.168.122.128",
				Port:   8848,
			},
		}
		client, err := clients.NewNamingClient(vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		})
		if err != nil {
			panic(err)
		}

		models, err := client.SelectAllInstances(vo.SelectAllInstancesParam{
			ServiceName: "user",
		})
		if err != nil {
			panic(err)
		}

		for _, model := range models {
			fmt.Println(model.Metadata)
		}

		client.Subscribe(&vo.SubscribeParam{
			ServiceName: "user",
			SubscribeCallback: func(services []model.SubscribeService, err error) {
				if err != nil {
					fmt.Println(err)
					return
				}
				for _, model := range services {
					fmt.Println(model.Metadata)
				}
			},
		})
		forever := make(chan struct{})
		<-forever
	}()

	forever := make(chan struct{})
	<-forever
}
```

问题：在上述的方案中，实例是通过定期调用注册接口来上报当前负载数据的，这样做有没有什么问题？
我的看法是，从实现机制上看我觉得是OK的。
从代码可读性上看，是否友好？
从性能上看，注册接口的性能开销是否小。
