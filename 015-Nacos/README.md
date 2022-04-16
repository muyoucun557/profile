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
容器启动之后，发现nacos尚未启动，经排查，nacos启动的时候会在``/home/nacos/logs``中创建新文件，出现了权限问题。








