# 已测版本示例：HAMi v2.10.0

本示例只用于复现 2026 年 10 月 30 日已测环境，不代表执行时的默认版本。它使用 Linux/amd64、K3s `v1.32.5+k3s1`、官方 Mokka v0.6.0 与 HAMi v2.10.0；未指定版本时，先按[环境准备](environment.md#选择兼容版本)查询两者的最新正式版。任何其他版本组合都须重新核对和验证。示例 values 在 [assets](../assets/) 中；先阅读选定 Chart 的 values 和模板，再复制到任务工作目录修改。

## 创建专用集群

先确认名称未被占用。`hami_cluster` 必须替换成当前任务的唯一名称；后续所有命令使用同一 kubeconfig/context。

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

## 安装官方 Mokka

将 `hami_skill_dir` 设为本 Skill 的绝对目录。示例 [Mokka values](../assets/mokka-v0.6.0-t4x2.yaml) 配置两张 T4、worker 选择器、NRI，并固定官方镜像 digest。Mokka v0.6.0 的容器运行时自动配置默认开启，但 k3d K3s 节点未提供它要挂载的 `/usr/bin` 路径；此示例关闭自动配置，保留 K3s 已启用的 NRI。其他运行时须重新核对该设置。OCI Chart 包与镜像的 SHA-256 是两种不同对象，分别核对。

```bash
hami_skill_dir=/absolute/path/to/skill/hami-k3s-mokka-smoke
helm pull oci://ghcr.io/nvidia/k8s-test-infra/chart/nvml-mock \
  --version 0.6.0 --destination "$hami_run_dir"
sha256sum "$hami_run_dir/nvml-mock-0.6.0.tgz"
cp "$hami_skill_dir/assets/mokka-v0.6.0-t4x2.yaml" "$hami_run_dir/mokka-values.yaml"
helm lint "$hami_run_dir/nvml-mock-0.6.0.tgz" \
  -f "$hami_run_dir/mokka-values.yaml" --strict
helm template mokka "$hami_run_dir/nvml-mock-0.6.0.tgz" -n mokka \
  -f "$hami_run_dir/mokka-values.yaml" > "$hami_run_dir/mokka-rendered.yaml"
helm install mokka "$hami_run_dir/nvml-mock-0.6.0.tgz" -n mokka --create-namespace \
  --kubeconfig "$hami_run_dir/kubeconfig" --kube-context "$hami_context" \
  -f "$hami_run_dir/mokka-values.yaml" --wait --timeout 180s
```

## 安装匹配的 HAMi release

使用 [HAMi v2.10.0](https://github.com/Project-HAMi/HAMi/releases/tag/v2.10.0) 原版 Chart 与同版本应用镜像。[HAMi values](../assets/hami-v2.10.0-k3s-v1.32.5.yaml) 将 kube-scheduler 镜像设置为 `v1.32.5`，使用真实 NVIDIA device-plugin，并指向 Mokka driver root。Mokka v0.6.0 的 NRI 只向获分配 GPU 或显式声明的容器注入模拟驱动，因此示例为 device-plugin Pod 添加 `nvml-mock.nvidia.com/devices: "true"`；否则插件无法加载 NVML。只在 checkout 与 Chart metadata 符合预期时执行安装。

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

此示例预期 checkout 为 `4707fb02c91c545bc7343ce26dba4c32919f9a3e`，HAMi Chart `appVersion=2.10.0`；Mokka Chart `appVersion=0.6.0`，Chart OCI digest 为 `sha256:173ed1d79f7a3886214552e38a2d8029e4724546a0c9276a89e6cf578f0c5ca7`。若不匹配，先核对 tag 和来源。安装后按[环境准备的分层检查](environment.md#安装与分层检查)确认：Mokka 生成两设备配置，HAMi 注册两个不同 GPU UUID，逻辑容量由 `2 × deviceSplitCount(10)` 得到 20；不以这些数值证明调度或 Allocate。
