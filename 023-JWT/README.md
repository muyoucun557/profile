# JWT

官网介绍<https://jwt.io/introduction>
RFC<https://www.rfc-editor.org/info/rfc7519>

JSON Web Token (JWT)是一个开放标准(RFC 7519)，它定义了一种紧凑的、自包含的方式，用于作为JSON对象在各方之间安全地传输信息。该信息可以被验证和信任，因为它是数字签名的。

这里需要注意一下：jwt是数字签名的，不是加密的。加密需要密钥才能将密文解密成明文；数字签名保证信息的来源是可靠的，未被篡改的。