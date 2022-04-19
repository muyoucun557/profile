# PG扩展

执行``create extension plugin-name;``能安装对应的插件。

我打算在我本机PG中安装一个叫``ganos``的插件。执行``create extension ganos;``，PG给出的报错如下
```Text
ERROR:  could not open extension control file "/usr/pgsql-12/share/extension/ganos.control": No such file or directory
SQL state: 58P01
```

