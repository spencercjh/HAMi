# 准备隔离的 K3s + Mokka + HAMi 环境

本页只准备环境，不运行测试工作负载。2026 年 10 月在 Linux/amd64 上验证过 K3s `v1.32.5+k3s1`、[官方 Mokka v0.4.0](https://github.com/NVIDIA/k8s-test-infra/blob/v0.4.0/docs/guides/device-plugin.md) 和 [HAMi v2.10.0](https://github.com/Project-HAMi/HAMi/releases/tag/v2.10.0) 的组合。它不是对其他版本的兼容承诺；换版本时重新核对 [Mokka Chart](https://github.com/NVIDIA/k8s-test-infra/blob/v0.4.0/docs/helm-chart.md)、HAMi Chart、镜像和 K3s/containerd 的 NRI 支持。

## 预检与隔离

需要 Linux/amd64、Docker、k3d、kubectl、Helm，以及足够的磁盘和内存。先检查已有集群、工具版本和目标名称，确认可以创建一个任务专属集群。以下变量仅用于当前 shell；所有后续 `kubectl` 与 `helm` 操作都显式传入专用 kubeconfig/context，不切换默认 context。

```bash
hami_run_dir=$(mktemp -d)
hami_cluster=hami-mokka-unique-name
hami_context="k3d-$hami_cluster"
k3d cluster list
k3d cluster create "$hami_cluster" --image rancher/k3s:v1.32.5-k3s1 \
  --servers 1 --agents 1 \
  --k3s-node-label 'mokka.nvidia.com/type=sgpu@agent:0' \
  --k3s-node-label 'gpu=on@agent:0' \
  --kubeconfig-update-default=false --kubeconfig-switch-context=false
k3d kubeconfig get "$hami_cluster" > "$hami_run_dir/kubeconfig"
chmod 600 "$hami_run_dir/kubeconfig"
kubectl --kubeconfig "$hami_run_dir/kubeconfig" --context "$hami_context" get nodes -o wide
```

若集群名已存在，不复用或覆盖。若 NRI socket/配置与本例不同，按 [Mokka NRI 指南](https://github.com/NVIDIA/k8s-test-infra/blob/v0.4.0/docs/guides/node-wide-injection/README.md) 排查，不能仅凭 DaemonSet Ready 判断注入成功。

## 安装官方 Mokka

使用仓库内的 [T4 × 2 values](../assets/mokka-v0.4.0-t4x2.yaml)，并核对当前 Chart 的字段及渲染结果。该文件将官方镜像固定为 `ghcr.io/nvidia/nvml-mock:0.4.0@sha256:4e93522b4c2af5419e0dc3e92fcb461a630d9c16cfafce72e385348602e5ce0e`，显式启用 NRI。NRI overlay 可能影响同一节点上的其他容器，所以仅在隔离集群使用。

```bash
hami_skill_dir=/absolute/path/to/skill/hami-k3s-mokka-smoke
helm pull oci://ghcr.io/nvidia/k8s-test-infra/chart/nvml-mock \
  --version 0.4.0 --destination "$hami_run_dir"
sha256sum "$hami_run_dir/nvml-mock-0.4.0.tgz"
cp "$hami_skill_dir/assets/mokka-v0.4.0-t4x2.yaml" "$hami_run_dir/mokka-values.yaml"
helm lint "$hami_run_dir/nvml-mock-0.4.0.tgz" \
  -f "$hami_run_dir/mokka-values.yaml" --strict
helm template mokka "$hami_run_dir/nvml-mock-0.4.0.tgz" -n mokka \
  -f "$hami_run_dir/mokka-values.yaml" > "$hami_run_dir/mokka-rendered.yaml"
helm install mokka "$hami_run_dir/nvml-mock-0.4.0.tgz" -n mokka --create-namespace \
  --kubeconfig "$hami_run_dir/kubeconfig" --kube-context "$hami_context" \
  -f "$hami_run_dir/mokka-values.yaml" --wait --timeout 180s
```

渲染时确认 Mokka 与 NRI DaemonSet 只落在已标记 agent；安装后核对 Pod Ready、NRI 与 containerd 连接日志、agent 的两设备配置、`/var/lib/nvml-mock/driver` 和 `/dev/nvidia0`、`/dev/nvidia1`。安装 HAMi 前确认没有其他插件注册 `nvidia.com/gpu`。Mokka 管理 Pod 中的 `nvidia-smi` 可能显示 profile 原始卡数；以 agent 实际配置、设备节点及 HAMi 注册结果为准。

## 安装匹配版本的真实 HAMi

从 HAMi v2.10.0 release 源码取原版 Chart，与其 v2.10.0 应用镜像一起使用。不要把不断变化的 `master` Chart 与旧应用镜像混装：2026-10-30 的 `master` Chart 搭配 v2.10.0 镜像曾因新参数 `--report-node-capacity` 和 `--extender-bind` 无法启动。换用其他版本前先核对 Chart `appVersion`、镜像及参数。

本例 K3s 为 v1.32.5，因此 [HAMi values](../assets/hami-v2.10.0-k3s-v1.32.5.yaml) 将 kube-scheduler 镜像设为 v1.32.5，禁用 HAMi 的 mock device-plugin，启用真实 NVIDIA device-plugin，并指向 Mokka driver root。

```bash
git clone --depth 1 --branch v2.10.0 https://github.com/Project-HAMi/HAMi.git \
  "$hami_run_dir/hami-source"
git -C "$hami_run_dir/hami-source" rev-parse HEAD
helm show chart "$hami_run_dir/hami-source/charts/hami"
cp "$hami_skill_dir/assets/hami-v2.10.0-k3s-v1.32.5.yaml" "$hami_run_dir/hami-values.yaml"
helm lint "$hami_run_dir/hami-source/charts/hami" \
  -f "$hami_run_dir/hami-values.yaml" --strict
helm template hami "$hami_run_dir/hami-source/charts/hami" -n hami-system \
  -f "$hami_run_dir/hami-values.yaml" > "$hami_run_dir/hami-rendered.yaml"
helm install hami "$hami_run_dir/hami-source/charts/hami" \
  -n hami-system --create-namespace \
  --kubeconfig "$hami_run_dir/kubeconfig" --kube-context "$hami_context" \
  -f "$hami_run_dir/hami-values.yaml" --wait --timeout 180s
```

预期 release checkout 为 `4707fb02c91c545bc7343ce26dba4c32919f9a3e`，Chart `appVersion=2.10.0`；不匹配时停止并重新核对，不继续套用 values。渲染时确认 kube-scheduler 镜像、device-plugin 的 driver-root 挂载、`passDeviceSpecsEnabled` 和 nodeSelector。HAMi 安装在 `hami-system`，不要安装到 Mokka NRI 默认排除的 `kube-system`。若镜像拉取或进程 ABI 出错，记录实际 `imageID` 与日志，不静默替换版本。

## 就绪与交接

分别记录：Node Ready；Mokka agent/NRI 就绪和两设备配置；HAMi webhook/scheduler、真实 NVIDIA device-plugin/monitor 就绪；Node 注册的 GPU UUID 与容量。此基线预期是两个不同 T4 UUID、每张 `deviceSplitCount=10`，共 20 个逻辑 `nvidia.com/gpu` 槽位。若只完成这些检查，只能称环境就绪和注册通过，不能称调度或 Allocate 通过。

交接集群名、专用 kubeconfig **路径**及 context、两个 release/namespace、版本与镜像 digest、values 副本、Node UUID/容量、实际 `imageID`、日志异常和未验证项。不要上传 kubeconfig 正文或其他凭据。默认保留环境给后续测试使用；需要验证时再读[可选回归](validation.md)。

## 按需清理

只有明确要求销毁，或确认本次环境生命周期结束且无人使用时才清理。先复核集群归属和名称，保存需要的证据；用专用 kubeconfig/context 卸载本次安装的 HAMi 和 Mokka release，等相关 Pod 退出后再删本次创建的 k3d 集群及专用 kubeconfig。等待超时则保留现场排查，不强制删除未知资源，不操作默认 kubeconfig、共享镜像或卷。
