## Test preAlpha version EDX 

This is detailed description for how two test EDX yourself

[![API Reference](
https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667
)](https://godoc.org/github.com/ethereum/go-ethereum)
[![Go Report Card](https://goreportcard.com/badge/github.com/ethereum/go-ethereum)](https://goreportcard.com/report/github.com/ethereum/go-ethereum)
[![Travis](https://travis-ci.org/ethereum/go-ethereum.svg?branch=master)](https://travis-ci.org/ethereum/go-ethereum)
[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://discord.gg/nthXNEv)

Anyone can test high throughout of EDX, this article shows how to test yourself step by step.

说明
====

PreAlpha版本的代码还没有完全开源，您可以到[此处](https://github.com/EDXFund/MasterChain/releases)下载可执行文件，或者发送邮件给[我们](mailto://pluto.shu@gmail.com)，请求获得完整代码和编译步骤。


运行
====

   **在运行之前 ，首先需要明确以下几项：**
> * *TPS能力和分片数量有关系，每个分片为2900笔交易，以15秒为出块单位，单片的TPS能力为195TPS*
> * *测试文件里，目前默认设置为4个分片，约合800TPS的能力*
 > * *TPS能力受到网络带宽，主链节点CPU运算能力的约束，在性能低的TPS上，设置较大的分片数可能会导致系统无法处理*
 > * *目前在prealpha中，峰值TPS已经达到2400*
   

#### 1) 下载Edx-PreAlpha版本
* [window版本下载](https://github.com/EDXFund/MasterChain/releases/download/v1.0.0-alpha/edx-windows64-v1.0.0-alpha.zip)
* [linux版本下载](https://github.com/EDXFund/MasterChain/releases/download/v1.0.0-alpha/edx-linux64-v1.0.0-alpha.zip)


#### 2) 编写配置文件
   该测试程序需要暂用系统端口：8082(区块浏览器http服务), 8547~8567（节点状态dashboard服务） ,3035～3050（p2p服务）
   钱包默认mnemonic："whip matter defense behave advance boat belt purse oil hamster stable clump"



#### 3) 启动节点
```
直接运行可执行文件main或mian.exe
```


#### 4) 区块浏览器 

```
打开浏览器，在地址栏中输入http://localhost:8082/
```
* 区块浏览器

![QQ20181227-170432@2x.png](https://upload-images.jianshu.io/upload_images/764896-ee2d037c4e590a9f.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/700)

* 主网监控

![QQ20181227-170454@2x.png](https://upload-images.jianshu.io/upload_images/764896-028a0e2f3bf02998.png?imageMogr2/auto-orient/strip%7CimageView2/2/w/700)

使用进阶
====

#### 生成交易
使用[edx.js](https://github.com/EDXFund/edx.js)库向主网发送交易请求，服务提供器地址：ws://127.0.0.1:8548。
```
/*获取最新区块信息*/
edx.mainsubscribe('newBlockHeaders');
/*获取区块信息*/
edx.main.getBlock(blockHashOrNumber);
/*获取分片信息*/
edx.main.getShardBlockByHash(shardHash,shardId);
/*发送单笔交易*/
edx.main.sendSignedTransaction();
```

#### 查看交易打包情况
```
/*发送交易详情*/
edx.main.getTransaction
```

