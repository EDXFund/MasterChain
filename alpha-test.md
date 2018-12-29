## Test preAlpha version EDX 

This is detailed description for how two test EDX yourself

[![API Reference](
https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667
)](https://godoc.org/github.com/ethereum/go-ethereum)
[![Go Report Card](https://goreportcard.com/badge/github.com/ethereum/go-ethereum)](https://goreportcard.com/report/github.com/ethereum/go-ethereum)
[![Travis](https://travis-ci.org/ethereum/go-ethereum.svg?branch=master)](https://travis-ci.org/ethereum/go-ethereum)
[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://discord.gg/nthXNEv)

Anyone can test high throughout of EDX, this article shows how to test yourself step by step.

## 下载可执行文件 

PreAlpha版本的代码还没有完全开源，您可以到[此处](https://github.com/EDXFund/MasterChain/releases)下载liunx与windows可执行文件，或者发送邮件给[我们](mailto://pluto.shu@gmail.com)，请求获得完整代码和编译步骤


## 运行

   在运行之前 ，首先需要明确以下几项：
   
   1) TPS能力和分片数量有关系，每个分片为2900笔交易，以15秒为出块单位，单片的TPS能力为195TPS
   2) 测试文件里，目前默认设置为4个分片，约合800TPS的能力
   3) TPS能力受到网络带宽，主链节点CPU运算能力的约束，在性能低的TPS上，设置较大的分片数可能会导致系统无法处理
   4) 目前在prealpha中，峰值TPS已经达到2400
   

### 下载Edx-Prealpha版本
   现提供linux与windows的64位编译测试版本。文件包括测试执行文件main与区块浏览器web目录。


### 编写配置文件
   该测试程序需要暂用系统端口：8082(区块浏览器http服务), 8547~8567（节点状态dashboard服务） ,3035～3050（p2p服务）
   钱包默认mnemonic："whip matter defense behave advance boat belt purse oil hamster stable clump"



### 启动节点
   直接运行可执行文件main或mian.exe


### 区块浏览器

 点击[这里](http://localhost:8082/)打开EDX的区块浏览器
```
 或者在浏览器地址栏中输入(http://localhost:8082/)
```

### 使用进阶
#### 生成交易
#### 查看交易打包情况

