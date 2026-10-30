# 在已交接环境中按需回归

仅在任务要求相应验证时执行。先取得环境负责人交接的专用 kubeconfig、context、values 与组件就绪证据；不复用未知集群，也不因测试完成自动卸载。所有 `kubectl` 均显式指定专用 kubeconfig/context。

## 调度与 Allocate

在本次专属 namespace `hami-test` 中应用[单容器测试 Pod](../assets/share-pod.yaml)。Pod 限制为 `nvidia.com/gpu: 1` 与 `nvidia.com/gpumem: 7000`；显存单位为 MiB。先核对测试镜像 digest 与架构，替换镜像时记录新 digest。

```bash
kubectl --kubeconfig "$hami_run_dir/kubeconfig" --context "$hami_context" \
  create namespace hami-test
kubectl --kubeconfig "$hami_run_dir/kubeconfig" --context "$hami_context" \
  apply -f "$hami_skill_dir/assets/share-pod.yaml"
kubectl --kubeconfig "$hami_run_dir/kubeconfig" --context "$hami_context" \
  -n hami-test get pod share-1 -o wide
```

分层核对并留存 Pod/Node JSON、事件、scheduler 与 device-plugin 日志、Pod `imageID`：

- Mokka 两设备配置和设备节点；HAMi Node 上两个不同 GPU UUID、逻辑容量 `10 × 2 = 20`。
- scheduler 的分配注解和目标节点；真实 device-plugin 的 Allocate 返回同一 UUID、`CUDA_DEVICE_MEMORY_LIMIT_0=7000m`、HAMi 库挂载及模拟设备的 DeviceSpec。
- 容器内 `NVIDIA_VISIBLE_DEVICES`、`CUDA_DEVICE_MEMORY_LIMIT_0`、`LD_PRELOAD`、HAMi 库与 `/dev/nvidia*` 与分配一致。Pod `Running` 不能替代这些证据。

## 可选容量和恢复检查

- 容量：按当前卡数与显存计算预期，逐个提交同规格 Pod。本例两张 16 GiB 卡可各承载两个 7000 MiB Pod；第五个应因显存不足而 Pending。记录事件，删除本次创建的一个运行 Pod 后，确认第五个转为 Running。不要把逻辑槽位数误当成显存容量。
- 插件重启：先记录 UUID、capacity、旧 Pod UID/containerID；仅重启本次 HAMi device-plugin，确认登记未重复、旧容器未改变，再释放/申请一次触发新 Allocate。
- 故障注入：只有明确要求时才按 [Mokka 控制工具](https://github.com/NVIDIA/k8s-test-infra/blob/v0.4.0/docs/guides/failure-injection/README.md) 在本次环境注入、记录、重置故障；先制定恢复步骤。不同 watcher 可能竞争消费 Xid，结果不稳定时标为受限，不关闭健康检查制造通过。

分别报告驱动、注册、调度、Allocate、容器、容量、重启和故障的 `PASS`、`FAIL`、`BLOCKED` 或 `NOT TESTED`；未执行项必须标记 `NOT TESTED`。诊断时先看镜像拉取/ABI、NRI socket、namespace 排除、nodeSelector、driver root、资源名冲突、Chart 挂载、scheduler 版本和 Pod 事件。模拟回归不能证明 CUDA kernel 执行、真实 GPU 隔离、性能、MIG 或生产授权。清理只限本次创建的测试资源；集群归属及清理由环境负责人按[环境准备](environment.md#按需清理)确认。
