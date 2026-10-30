# 按任务验证功能

只有任务明确要求功能验证时执行对应小节。先取得环境交接信息，确认专用 kubeconfig/context、Mokka 与 HAMi 的版本和已就绪层级。测试资源放在任务专属 namespace；不复用未知集群。以下 [Pod 示例](../assets/share-pod.yaml) 使用 `hami-test` namespace、一个逻辑 GPU 和 7000 MiB 显存，仅适用于容量足够、资源名相同的环境。不同配置应调整请求并重新计算预期。

## 调度和 Allocate

先提交单个测试 Pod，等待调度结果，收集 Pod/Node JSON、事件、scheduler 和 device-plugin 日志、实际 `imageID`。分层判断：

1. **注册：**Mokka 的模拟设备配置、driver root 与设备节点一致；HAMi Node 注册不同的 GPU UUID，逻辑容量与当前 `deviceSplitCount` 一致。
2. **调度：**Pod 被 HAMi scheduler 绑定，分配注解中的节点、设备 UUID 和请求量一致。Pod `Running` 本身不代表调度证据充分。
3. **Allocate：**真实 device-plugin 日志返回同一 UUID、请求的显存限制、HAMi 库挂载及对应模拟设备的 DeviceSpec。没有该日志时不要声称 Allocate 通过。
4. **容器：**在容器内核对 `NVIDIA_VISIBLE_DEVICES`、`CUDA_DEVICE_MEMORY_LIMIT_0`、`LD_PRELOAD`、HAMi 库与 `/dev/nvidia*`，确认和分配一致。没有容器内证据时，只报告到 Allocate 层。

## 按需扩展

- **容量：**根据当前设备数和每卡显存计算可容纳的 Pod 数，再逐个提交。示例的两张 16 GiB T4 各可放两个 7000 MiB Pod；第五个应因显存不足等待。记录调度事件。仅删除本次创建的一个运行 Pod 后，确认等待 Pod 获得资源。逻辑 GPU 槽位数不等于显存容量。
- **插件重启：**先记录 UUID、容量、旧 Pod UID/containerID；只重启本次 HAMi device-plugin，确认注册没有重复、旧容器未改变，再触发新 Allocate。
- **故障注入：**仅在任务明确要求时，在 [Mokka failure-injection 指南目录](https://github.com/NVIDIA/k8s-test-infra/tree/main/docs/guides/failure-injection)切换到所选 release tag，阅读对应指南，再制定注入、留证和重置步骤。Xid 可能被多个 watcher 竞争消费；结果不稳定时如实标记，不关闭健康检查制造通过。

交付时按驱动、注册、调度、Allocate、容器、容量、重启和故障分别标记 `PASS`、`FAIL`、`BLOCKED` 或 `NOT TESTED`。记录版本、镜像 digest、预期与实际值、证据路径及失败原因；未执行项写 `NOT TESTED`。测试结束不自动卸载已交接环境。模拟环境不能证明真实 CUDA 执行、GPU 隔离或生产性能。
